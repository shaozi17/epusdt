package data

import (
	"github.com/assimon/luuu/model/dao"
	"github.com/assimon/luuu/model/mdb"
	"github.com/assimon/luuu/util/constant"
)

// AddWalletAddress 创建钱包
func AddWalletAddress(token string) (*mdb.WalletAddress, error) {
	exist, err := GetWalletAddressByToken(token)
	if err != nil {
		return nil, err
	}
	if exist.ID > 0 {
		return nil, constant.WalletAddressAlreadyExists
	}
	walletAddress := &mdb.WalletAddress{
		Token:  token,
		Status: mdb.TokenStatusEnable,
	}
	err = dao.Mdb.Create(walletAddress).Error
	return walletAddress, err
}

// GetWalletAddressByToken 通过钱包地址获取token
func GetWalletAddressByToken(token string) (*mdb.WalletAddress, error) {
	walletAddress := new(mdb.WalletAddress)
	err := dao.Mdb.Model(walletAddress).Limit(1).Find(walletAddress, "token = ?", token).Error
	return walletAddress, err
}

// GetWalletAddressById 通过id获取钱包
func GetWalletAddressById(id uint64) (*mdb.WalletAddress, error) {
	walletAddress := new(mdb.WalletAddress)
	err := dao.Mdb.Model(walletAddress).Limit(1).Find(walletAddress, id).Error
	return walletAddress, err
}

// DeleteWalletAddressById 通过id删除钱包
func DeleteWalletAddressById(id uint64) error {
	err := dao.Mdb.Where("id = ?", id).Delete(&mdb.WalletAddress{}).Error
	return err
}

// GetAvailableWalletAddress 获得所有可用的钱包地址
func GetAvailableWalletAddress() ([]mdb.WalletAddress, error) {
	var WalletAddressList []mdb.WalletAddress
	err := dao.Mdb.Model(WalletAddressList).Where("status = ?", mdb.TokenStatusEnable).Find(&WalletAddressList).Error
	return WalletAddressList, err
}

// GetPendingWalletAddress 获得订单正在进行中的钱包地址
func GetPendingWalletAddress() ([]mdb.WalletAddress, error) {
	var WalletAddressList []mdb.WalletAddress

	ordersTable := (&mdb.Orders{}).TableName()
	walletAddressTable := (&mdb.WalletAddress{}).TableName()

	err := dao.Mdb.Model(WalletAddressList).
		Where("EXISTS (SELECT 1 FROM "+ordersTable+" WHERE "+ordersTable+".token = "+walletAddressTable+".token AND "+ordersTable+".status = ?)", mdb.StatusWaitPay).
		Where("status = ?", mdb.TokenStatusEnable).
		Find(&WalletAddressList).Error
	return WalletAddressList, err
}

// GetSpecialWalletAddress 获得指定钱包地址
func GetSpecialWalletAddress(walletToken string) (WalletAddressList []mdb.WalletAddress, err error) {
	if walletToken == "" {
		return
	}

	err = dao.Mdb.Model(WalletAddressList).
		Where("token = ?", walletToken).
		Where("status = ?", mdb.TokenStatusEnable).
		Find(&WalletAddressList).
		Error
	if err != nil {
		return nil, err
	}
	if len(WalletAddressList) == 0 {
		newWalletAddress := &mdb.WalletAddress{
			Token:  walletToken,
			Status: mdb.TokenStatusEnable,
		}
		err = dao.Mdb.Create(newWalletAddress).Error
		if err != nil {
			return nil, err
		}
		WalletAddressList = append(WalletAddressList, *newWalletAddress)
	}
	return WalletAddressList, err
}

// GetAllWalletAddress 获得所有钱包地址
func GetAllWalletAddress() ([]mdb.WalletAddress, error) {
	var WalletAddressList []mdb.WalletAddress
	err := dao.Mdb.Model(WalletAddressList).Find(&WalletAddressList).Error
	return WalletAddressList, err
}

// ChangeWalletAddressStatus 启用禁用钱包
func ChangeWalletAddressStatus(id uint64, status int) error {
	err := dao.Mdb.Model(&mdb.WalletAddress{}).Where("id = ?", id).Update("status", status).Error
	return err
}
