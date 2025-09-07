package task

import "github.com/robfig/cron/v3"

func Start() {
	c := cron.New()
	// 汇率监听
	c.AddJob("@every 60s", UsdtRateJob{})
	// 监听金额订单
	c.AddJob("@every 5s", ListenJob{})
	// 监听用户提交了 Hash 的订单
	c.AddJob("@every 5s", ListenHashJob{})
	c.Start()
}
