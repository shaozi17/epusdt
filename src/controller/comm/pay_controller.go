package comm

import (
	"fmt"
	"html/template"
	"net/http"

	"github.com/assimon/luuu/config"
	"github.com/assimon/luuu/model/request"
	"github.com/assimon/luuu/model/response"
	"github.com/assimon/luuu/model/service"
	"github.com/assimon/luuu/mq"
	"github.com/assimon/luuu/mq/handle"
	"github.com/hibiken/asynq"
	"github.com/labstack/echo/v4"
)

// CheckoutCounter 收银台
func (c *BaseCommController) CheckoutCounter(ctx echo.Context) (err error) {
	tradeId := ctx.Param("trade_id")
	resp, err := service.GetCheckoutCounterByTradeId(tradeId)
	if err != nil {
		return ctx.String(http.StatusOK, err.Error())
	}
	tmpl, err := template.ParseFiles(fmt.Sprintf(".%s/%s", config.StaticPath, "index.html"))
	if err != nil {
		return ctx.String(http.StatusOK, err.Error())
	}
	return tmpl.Execute(ctx.Response(), resp)
}

// CheckStatus 支付状态检测
func (c *BaseCommController) CheckStatus(ctx echo.Context) (err error) {
	tradeId := ctx.Param("trade_id")
	order, err := service.GetOrderInfoByTradeId(tradeId)
	if err != nil {
		return c.FailJson(ctx, err)
	}
	resp := response.CheckStatusResponse{
		TradeId: order.TradeId,
		Status:  order.Status,
	}
	return c.SucJson(ctx, resp)
}

// CheckHashStatus Hash订单状态检测
func (c *BaseCommController) CheckHashStatus(ctx echo.Context) (err error) {
	tradeId := ctx.Param("trade_id")
	txHash := ctx.Param("txHash")

	req := &request.OrderProcessingRequest{
		TradeId:            tradeId,
		BlockTransactionId: txHash,
	}
	order, err := service.OrderUpdateTxid(req)
	if err != nil {
		return c.FailJson(ctx, err)
	}

	resp := response.CheckHashStatusResponse{
		TradeId:            order.TradeId,
		Status:             order.Status,
		BlockTransactionId: order.BlockTransactionId,
	}

	// 回调队列 更新交易哈希
	orderCallbackQueue, _ := handle.NewOrderCallbackQueue(order)
	mq.MClient.Enqueue(orderCallbackQueue, asynq.MaxRetry(5))
	return c.SucJson(ctx, resp)
}
