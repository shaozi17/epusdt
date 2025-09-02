package task

import (
	"sync"

	"github.com/assimon/luuu/model/data"
	"github.com/assimon/luuu/model/service"
	"github.com/assimon/luuu/util/log"
)

type ListenJob struct {
}

var gListenJobLock sync.Mutex

func (r ListenJob) Run() {
	gListenJobLock.Lock()
	defer gListenJobLock.Unlock()
	walletAddress, err := data.GetPendingWalletAddress()
	if err != nil {
		log.Sugar.Error(err)
		return
	}
	if len(walletAddress) <= 0 {
		return
	}

	// 简单校验是否为 TRC20 地址（T + 34 个十六进制字符）
	isValidTRC20 := func(s string) bool {
		if s == "" {
			return false
		}
		if len(s) != 34 {
			return false
		}
		if s[0] != 'T' {
			return false
		}
		return true
	}
	// 简单校验是否为以太坊地址（0x + 40 个十六进制字符）
	isValidEth := func(s string) bool {
		if len(s) != 42 {
			return false
		}
		if !(s[0] == '0' && (s[1] == 'x' || s[1] == 'X')) {
			return false
		}
		for _, c := range s[2:] {
			if !((c >= '0' && c <= '9') || (c >= 'a' && c <= 'f') || (c >= 'A' && c <= 'F')) {
				return false
			}
		}
		return true
	}

	var wg sync.WaitGroup
	for _, address := range walletAddress {
		if isValidTRC20(address.Token) {
			wg.Add(1)
			go service.Trc20CallBack(address.Token, &wg)
		} else if isValidEth(address.Token) {
			wg.Add(1)
			go service.Erc20CallBack(address.Token, &wg)
		}
	}
	wg.Wait()
}
