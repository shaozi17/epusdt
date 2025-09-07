package service

import (
	"net/http"
	"sync"

	"github.com/assimon/luuu/model/data"
	"github.com/assimon/luuu/model/request"
	"github.com/assimon/luuu/mq"
	"github.com/assimon/luuu/mq/handle"
	"github.com/assimon/luuu/util/http_client"
	"github.com/assimon/luuu/util/json"
	"github.com/assimon/luuu/util/log"
	"github.com/hibiken/asynq"
	"github.com/shopspring/decimal"
)

const UsdtTrc20HashApiUri = "https://apilist.tronscanapi.com/api/transaction-info"

type UsdtTrc20HashResp struct {
	ContractRet       string              `json:"contractRet"`
	Trc20TransferInfo []Trc20TransferInfo `json:"trc20TransferInfo"`
}
type Trc20TransferInfo struct {
	Symbol      string `json:"symbol"`
	ToAddress   string `json:"to_address"`
	FromAddress string `json:"from_address"`
	AmountStr   string `json:"amount_str"`
	TokenType   string `json:"token_type"`
	Type        string `json:"type"`
}

// Trc20CallBack trc20回调
func Trc20HashCallBack(tradeId string, token string, hashId string, wg *sync.WaitGroup) {
	defer wg.Done()
	defer func() {
		if err := recover(); err != nil {
			log.Sugar.Error(err)
		}
	}()
	client := http_client.GetHttpClient()
	resp, err := client.R().SetQueryParams(map[string]string{
		"hash": hashId,
	}).Get(UsdtTrc20HashApiUri)
	if err != nil {
		panic(err)
	}
	if resp.StatusCode() != http.StatusOK {
		panic("resp.StatusCode() != http.StatusOK")
	}
	var trc20Resp UsdtTrc20HashResp
	err = json.Cjson.Unmarshal(resp.Body(), &trc20Resp)
	if err != nil {
		panic(err)
	}
	if len(trc20Resp.Trc20TransferInfo) <= 0 || trc20Resp.ContractRet != "SUCCESS" {
		return
	}
	for _, transfer := range trc20Resp.Trc20TransferInfo {
		if transfer.ToAddress != token || transfer.Symbol != "USDT" || transfer.Type != "Transfer" || transfer.TokenType != "trc20" {
			continue
		}
		decimalQuant, err := decimal.NewFromString(transfer.AmountStr)
		if err != nil {
			panic(err)
		}
		decimalDivisor := decimal.NewFromFloat(1000000)
		amount := decimalQuant.Div(decimalDivisor).InexactFloat64()
		// tradeId, err := data.GetTradeIdByWalletAddressAndAmount(token, amount)
		// if err != nil {
		// 	panic(err)
		// }
		// if tradeId == "" {
		// 	continue
		// }
		order, err := data.GetOrderInfoByTradeId(tradeId)
		if err != nil {
			panic(err)
		}
		// 区块的确认时间必须在订单创建时间之后
		// createTime := order.CreatedAt.TimestampWithMillisecond()
		// if transfer.BlockTimestamp < createTime {
		// 	panic("Orders cannot actually be matched")
		// }
		// 到这一步就完全算是支付成功了
		req := &request.OrderProcessingRequest{
			Token:              token,
			TradeId:            tradeId,
			Amount:             amount,
			BlockTransactionId: hashId,
		}
		err = OrderProcessing(req)
		if err != nil {
			panic(err)
		}
		// 回调队列
		orderCallbackQueue, _ := handle.NewOrderCallbackQueue(order)
		mq.MClient.Enqueue(orderCallbackQueue, asynq.MaxRetry(5))
	}
}
