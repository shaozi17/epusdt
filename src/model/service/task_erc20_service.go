package service

import (
	"fmt"
	"net/http"
	"strconv"
	"sync"

	"github.com/assimon/luuu/model/data"
	"github.com/assimon/luuu/model/request"
	"github.com/assimon/luuu/mq"
	"github.com/assimon/luuu/mq/handle"
	"github.com/assimon/luuu/telegram"
	"github.com/assimon/luuu/util/http_client"
	"github.com/assimon/luuu/util/json"
	"github.com/assimon/luuu/util/log"
	"github.com/golang-module/carbon/v2"
	"github.com/hibiken/asynq"
	"github.com/shopspring/decimal"
)

const UsdtErc20ApiUri = "https://blockscout.com/eth/mainnet/api"
const UsdtErc20ApiKey = "0135db27-4208-47b4-b757-e61030334a96"
const UsdtErc20Contract = "0xdAC17F958D2ee523a2206206994597C13D831ec7"

type BlockscoutResp struct {
	Status  string      `json:"status"`
	Message string      `json:"message"`
	Result  []Erc20Data `json:"result"`
}

type Erc20Data struct {
	BlockNumber     string `json:"blockNumber"`
	TimeStamp       string `json:"timeStamp"`
	Hash            string `json:"hash"`
	From            string `json:"from"`
	To              string `json:"to"`
	ContractAddress string `json:"contractAddress"`
	Value           string `json:"value"`
	TokenName       string `json:"tokenName"`
	TokenSymbol     string `json:"tokenSymbol"`
	TokenDecimal    string `json:"tokenDecimal"`
	Confirmations   string `json:"confirmations"`
}

// Erc20CallBack 使用 Blockscout 查询 ERC20 (USDT) 交易记录并按原先逻辑处理
func Erc20CallBack(token string, wg *sync.WaitGroup) {
	defer wg.Done()
	defer func() {
		if err := recover(); err != nil {
			log.Sugar.Error(err)
		}
	}()
	client := http_client.GetHttpClient()

	// 查询最近 N 笔交易（按 Blockscout API）
	resp, err := client.R().SetQueryParams(map[string]string{
		"module":          "account",
		"action":          "tokentx",
		"address":         token,
		"contractaddress": UsdtErc20Contract,
		"page":            "1",
		"offset":          "100",
		"sort":            "desc",
		"apikey":          UsdtErc20ApiKey,
	}).Get(UsdtErc20ApiUri)
	if err != nil {
		panic(err)
	}
	if resp.StatusCode() != http.StatusOK {
		panic("blockscout api status not ok")
	}

	var bsResp BlockscoutResp
	err = json.Cjson.Unmarshal(resp.Body(), &bsResp)
	if err != nil {
		panic(err)
	}
	if bsResp.Status != "1" || len(bsResp.Result) == 0 {
		// 没有结果或查询失败，直接返回
		return
	}

	for _, t := range bsResp.Result {
		// 只处理发往我们监控的钱包地址
		if t.To != token {
			continue
		}
		// 解析 token decimals & value
		decimalsInt := 0
		if t.TokenDecimal != "" {
			if di, err := strconv.Atoi(t.TokenDecimal); err == nil {
				decimalsInt = di
			} else {
				// 不能解析 decimals，跳过
				continue
			}
		}

		valDecimal, err := decimal.NewFromString(t.Value)
		if err != nil {
			panic(err)
		}
		// 将整数 value 根据 decimals 转换为真实数值
		amountDecimal := valDecimal.Shift(-int32(decimalsInt))
		amount := amountDecimal.InexactFloat64()

		// 根据 wallet address 和 amount 查找 tradeId
		tradeId, err := data.GetTradeIdByWalletAddressAndAmount(token, amount)
		if err != nil {
			panic(err)
		}
		if tradeId == "" {
			continue
		}

		order, err := data.GetOrderInfoByTradeId(tradeId)
		if err != nil {
			panic(err)
		}

		// Blockscout 返回的 timeStamp 通常为秒级字符串，转换为毫秒
		tsSec := int64(0)
		if t.TimeStamp != "" {
			if s, err := strconv.ParseInt(t.TimeStamp, 10, 64); err == nil {
				tsSec = s
			} else {
				// 无法解析时间戳，跳过
				continue
			}
		}
		blockTimestampMillis := tsSec * 1000

		// 区块的确认时间必须在订单创建时间之后
		createTime := order.CreatedAt.TimestampWithMillisecond()
		if blockTimestampMillis < createTime {
			panic("Orders cannot actually be matched")
		}

		// 到这一步就完全算是支付成功了
		req := &request.OrderProcessingRequest{
			Token:              token,
			TradeId:            tradeId,
			Amount:             amount,
			BlockTransactionId: t.Hash,
		}
		err = OrderProcessing(req)
		if err != nil {
			panic(err)
		}

		// 回调队列
		orderCallbackQueue, _ := handle.NewOrderCallbackQueue(order)
		mq.MClient.Enqueue(orderCallbackQueue, asynq.MaxRetry(5))

		// 发送机器人消息
		msgTpl := `
<b>📢📢有新的交易支付成功！</b>
<pre>交易号：%s</pre>
<pre>订单号：%s</pre>
<pre>请求支付金额：%f cny</pre>
<pre>实际支付金额：%f usdt</pre>
<pre>钱包地址：%s</pre>
<pre>订单创建时间：%s</pre>
<pre>支付成功时间：%s</pre>
`
		msg := fmt.Sprintf(msgTpl, order.TradeId, order.OrderId, order.Amount, order.ActualAmount, order.Token, order.CreatedAt.ToDateTimeString(), carbon.Now().ToDateTimeString())
		telegram.SendToBot(msg)
	}
}
