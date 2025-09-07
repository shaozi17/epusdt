package service

import (
	"math"
	"math/big"
	"net/http"
	"strings"
	"sync"

	"github.com/assimon/luuu/model/data"
	"github.com/assimon/luuu/model/request"
	"github.com/assimon/luuu/mq"
	"github.com/assimon/luuu/mq/handle"
	"github.com/assimon/luuu/util/http_client"
	"github.com/assimon/luuu/util/json"
	"github.com/assimon/luuu/util/log"
	"github.com/hibiken/asynq"
)

type BlockscoutHashResp struct {
	Status  string        `json:"status"`
	Message string        `json:"message"`
	Result  Erc20HashData `json:"result"`
}

type Erc20HashData struct {
	BlockNumber   string `json:"blockNumber"`
	Confirmations string `json:"confirmations"`
	From          string `json:"from"`
	To            string `json:"to"`
	TimeStamp     string `json:"timeStamp"`
	Hash          string `json:"hash"`
	Value         string `json:"value"`
	Success       bool   `json:"success"`
	Logs          []Log  `json:"logs"`
}

type Log struct {
	Address     string   `json:"address"`
	Data        string   `json:"data"`
	Index       string   `json:"index"`
	Topics      []string `json:"topics"`
	BlockNumber string   `json:"blockNumber"`
}

// Erc20HashCallBack 使用 Blockscout 查询 ERC20 (USDT) 交易记录并按原先逻辑处理
func Erc20HashCallBack(tradeId string, token string, hashId string, wg *sync.WaitGroup) {
	defer wg.Done()
	defer func() {
		if err := recover(); err != nil {
			log.Sugar.Error(err)
		}
	}()
	client := http_client.GetHttpClient()

	// 查询最近 N 笔交易（按 Blockscout API）
	resp, err := client.R().SetQueryParams(map[string]string{
		"module": "transaction",
		"action": "gettxinfo",
		"txhash": hashId,
		"apikey": UsdtErc20ApiKey,
	}).Get(UsdtErc20ApiUri)
	if err != nil {
		panic(err)
	}
	if resp.StatusCode() != http.StatusOK {
		panic("blockscout api status not ok")
	}

	var bsResp BlockscoutHashResp
	err = json.Cjson.Unmarshal(resp.Body(), &bsResp)
	if err != nil {
		panic(err)
	}
	if bsResp.Status != "1" {
		// 没有结果或查询失败，直接返回
		return
	}

	t := bsResp.Result

	// 只监控 USDT 和 USDC
	if strings.ToLower(t.To) != strings.ToLower(UsdtErc20Contract) && strings.ToLower(t.To) != strings.ToLower(UsdcErc20Contract) {
		return
	}
	// 获取 USD 金额 & 对比收款地址
	amount := 0.0

	// 解析 logs 寻找 Transfer 事件并抽取 to 和 value
	// ERC20 Transfer 事件 signature
	const transferSig = "0xddf252ad1be2c89b69c2b068fc378daa952ba7f163c4a11628f55a4df523b3ef"

	// 常见 stablecoin decimals（按合约地址设置）
	decimalsMap := map[string]int{
		strings.ToLower(UsdtErc20Contract): 6,
		strings.ToLower(UsdcErc20Contract): 6,
	}

	var receiverAddr string
	var tokenAmount float64
	found := false

	for _, l := range t.Logs {
		if len(l.Topics) == 0 {
			continue
		}
		if strings.ToLower(l.Topics[0]) != transferSig {
			continue
		}
		if len(l.Topics) < 3 {
			continue
		}

		// topics[2] is indexed "to" (32 bytes hex). 取最后40字符作为地址
		toTopic := strings.TrimPrefix(l.Topics[2], "0x")
		if len(toTopic) < 40 {
			// 非标准格式，跳过
			continue
		}
		addrHex := "0x" + strings.ToLower(toTopic[len(toTopic)-40:])

		// data 字段为转账数额的 uint256 hex
		valHex := strings.TrimPrefix(l.Data, "0x")
		if valHex == "" {
			continue
		}
		valBig := new(big.Int)
		_, ok := valBig.SetString(valHex, 16)
		if !ok {
			continue
		}

		// 决定 decimals：优先用 log.Address（token 合约），否则使用 tx.to
		decimals := 6
		if d, ok := decimalsMap[strings.ToLower(l.Address)]; ok {
			decimals = d
		} else if d, ok := decimalsMap[strings.ToLower(t.To)]; ok {
			decimals = d
		}

		fVal := new(big.Float).SetInt(valBig)
		denom := new(big.Float).SetFloat64(math.Pow10(decimals))
		fVal.Quo(fVal, denom)
		fv64, _ := fVal.Float64()

		receiverAddr = addrHex
		tokenAmount = fv64
		found = true
		break
	}

	if !found {
		// 没有找到 transfer log，直接返回
		return
	}

	order, err := data.GetOrderInfoByTradeId(tradeId)
	if err != nil {
		panic(err)
	}

	if strings.ToLower(receiverAddr) != strings.ToLower(token) {
		// 收款地址不是目标地址，忽略
		return
	}

	// 对于 USDT/USDC 等稳定币，tokenAmount 近似等于 USD 数量
	amount = tokenAmount

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
}
