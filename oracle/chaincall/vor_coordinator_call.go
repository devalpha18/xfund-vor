package chaincall

import (
	"context"
	"crypto/ecdsa"
	"github.com/ethereum/go-ethereum/accounts/abi/bind"
	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/common/hexutil"
	"github.com/ethereum/go-ethereum/core/types"
	"github.com/ethereum/go-ethereum/crypto"
	"github.com/ethereum/go-ethereum/ethclient"
	"log"
	"math/big"
	"oracle/config"
	"oracle/contracts/vor_coordinator"
	"oracle/utils"
	"oracle/utils/walletworker"
)

type VORCoordinatorCaller struct {
	contractAddress common.Address
	client          *ethclient.Client
	instance        *vor_coordinator.VORCoordinator
	transactOpts    *bind.TransactOpts
	callOpts        *bind.CallOpts

	context          context.Context
	publicProvingKey [2]*big.Int
	oraclePrivateKey string
	oraclePublicKey  string
	oracleAddress    string
}

func NewVORCoordinatorCaller(contractStringAddress string, ethHostAddress string, chainID *big.Int, oraclePrivateKey []byte) (*VORCoordinatorCaller, error) {
	client, err := ethclient.Dial(ethHostAddress)
	ctx := context.Background()
	if err != nil {
		return nil, err
	}
	//fmt.Println("contractStringAddress: ", contractStringAddress)
	contractAddress := common.HexToAddress(contractStringAddress)
	instance, err := vor_coordinator.NewVORCoordinator(contractAddress, client)
	if err != nil {
		return nil, err
	}
	oraclePrivateKeyECDSA, err := crypto.HexToECDSA(utils.RemoveHexPrefix(string(oraclePrivateKey)))
	if err != nil {
		return nil, err
	}

	oraclePublicKey := oraclePrivateKeyECDSA.Public()
	//log.Print("Public Key: ", hexutil.Encode(crypto.FromECDSAPub(oraclePublicKey.(*ecdsa.PublicKey))))

	ECDSAoraclePublicKey, err := crypto.UnmarshalPubkey(crypto.FromECDSAPub(oraclePublicKey.(*ecdsa.PublicKey)))
	if err != nil || ECDSAoraclePublicKey == nil {
		log.Print(err)
		log.Print(ECDSAoraclePublicKey)
		return nil, err
	}
	_, oracleAddress := walletworker.GenerateAddress(ECDSAoraclePublicKey)
	//log.Print("Address: ", oracleAddress)

	transactOpts, err := bind.NewKeyedTransactorWithChainID(oraclePrivateKeyECDSA, chainID)
	if err != nil {
		return nil, err
	}

	gasPrice, err := client.SuggestGasPrice(ctx)
	if err != nil {
		return nil, err
	}
	nonce, err := client.PendingNonceAt(ctx, common.HexToAddress(oracleAddress))
	if err != nil {
		return nil, err
	}
	transactOpts.Nonce = big.NewInt(int64(nonce))
	transactOpts.Value = big.NewInt(0)
	transactOpts.GasPrice = gasPrice
	transactOpts.GasLimit = uint64(config.Conf.LimitGasPrice) // in units
	transactOpts.Context = ctx

	callOpts := &bind.CallOpts{From: common.HexToAddress(oracleAddress), Context: ctx}

	return &VORCoordinatorCaller{
		client:           client,
		contractAddress:  contractAddress,
		instance:         instance,
		transactOpts:     transactOpts,
		callOpts:         callOpts,
		context:          ctx,
		publicProvingKey: [2]*big.Int{ECDSAoraclePublicKey.X, ECDSAoraclePublicKey.Y},
		oraclePrivateKey: string(oraclePrivateKey),
		oraclePublicKey:  hexutil.Encode(crypto.FromECDSAPub(oraclePublicKey.(*ecdsa.PublicKey))),
		oracleAddress:    oracleAddress,
	}, err
}

func (d *VORCoordinatorCaller) RenewTransactOpts() (err error) {
	nonce, err := d.client.PendingNonceAt(d.context, common.HexToAddress(d.oracleAddress))
	if err != nil {
		return
	}
	d.transactOpts.Nonce = big.NewInt(int64(nonce))

	return
}

func (d *VORCoordinatorCaller) GetTotalGasDeposits() (*big.Int, error) {
	defer d.RenewTransactOpts()
	return d.instance.GetTotalGasDeposits(d.callOpts)
}

func (d *VORCoordinatorCaller) GetGasTopUpLimit() (*big.Int, error) {
	defer d.RenewTransactOpts()
	return d.instance.GetGasTopUpLimit(d.callOpts)
}

func (d *VORCoordinatorCaller) HashOfKey() ([32]byte, error) {
	defer d.RenewTransactOpts()
	return d.instance.HashOfKey(d.callOpts, d.publicProvingKey)
}

//func (d *VORCoordinatorCallerr) HashOfKeyLocally() ([]byte, error) {
//	utils.Keccak256(d.publicProvingKey)
//	crypto.Keccak256()
//}
//func (d *VORCoordinatorCaller) GetProviderAddress(bindOpts bind.CallOpts, keyHash string) (common.Address, error) {
//	return d.instance.GetProviderAddress(&bindOpts, [32]byte(keyHash))
//}

func (d *VORCoordinatorCaller) Withdraw(recipientAddress string, amount *big.Int) (*types.Transaction, error) {
	defer d.RenewTransactOpts()
	recipientAddr := common.HexToAddress(recipientAddress)
	return d.instance.Withdraw(d.transactOpts, recipientAddr, amount)
}

func (d *VORCoordinatorCaller) RegisterProvingKey(fee *big.Int, providerPaysGas bool) (*types.Transaction, error) {
	defer d.RenewTransactOpts()
	transaction, err := d.instance.RegisterProvingKey(d.transactOpts, fee, common.HexToAddress(d.oracleAddress), d.publicProvingKey, providerPaysGas)
	return transaction, err
}

func (d *VORCoordinatorCaller) RandomnessRequest(keyHash [32]byte, consumerSeed *big.Int, feePaid *big.Int) (*types.Transaction, error) {
	defer d.RenewTransactOpts()
	transaction, err := d.instance.RandomnessRequest(d.transactOpts, keyHash, consumerSeed, feePaid)
	return transaction, err
}

func (d *VORCoordinatorCaller) ChangeFee(fee *big.Int) (*types.Transaction, error) {
	defer d.RenewTransactOpts()
	return d.instance.ChangeFee(d.transactOpts, d.publicProvingKey, fee)
}

func (d *VORCoordinatorCaller) TopUpGas(gas *big.Int) (*types.Transaction, error) {
	defer d.RenewTransactOpts()
	d.transactOpts.Value = gas
	return d.instance.TopUpGas(d.transactOpts, common.HexToAddress(d.oracleAddress))
}

func (d *VORCoordinatorCaller) SetProviderPaysGas(providerPaysFee bool) (*types.Transaction, error) {
	defer d.RenewTransactOpts()
	return d.instance.SetProviderPaysGas(d.transactOpts, d.publicProvingKey, providerPaysFee)
}

func (d *VORCoordinatorCaller) FulfillRandomnessRequest(proof []byte) (*types.Transaction, error) {
	defer d.RenewTransactOpts()
	return d.instance.FulfillRandomnessRequest(d.transactOpts, proof)
}
