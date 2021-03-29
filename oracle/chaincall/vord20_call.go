package chaincall

import (
	"context"
	"crypto/ecdsa"
	"fmt"
	"github.com/ethereum/go-ethereum/accounts/abi/bind"
	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/common/hexutil"
	"github.com/ethereum/go-ethereum/core/types"
	"github.com/ethereum/go-ethereum/crypto"
	"github.com/ethereum/go-ethereum/ethclient"
	"log"
	"math/big"
	"oracle/contracts/vord_20"
	"oracle/utils/walletworker"
)

type VORD20Caller struct {
	contractAddress common.Address
	client          *ethclient.Client
	instance        *vord20.VORD20
	transactOpts    *bind.TransactOpts
	callOpts        *bind.CallOpts

	publicProvingKey [2]*big.Int
	oraclePrivateKey string
	oraclePublicKey  string
	oracleAddress    string
}

func NewVORD20Caller(contractStringAddress string, ethHostAddress string, chainID *big.Int, oraclePrivateKey []byte) (*VORD20Caller, error) {
	client, err := ethclient.Dial(ethHostAddress)
	if err != nil {
		return nil, err
	}
	fmt.Println("contractStringAddress: ", contractStringAddress)
	contractAddress := common.HexToAddress(contractStringAddress)
	instance, err := vord20.NewVORD20(contractAddress, client)
	if err != nil {
		return nil, err
	}
	oraclePrivateKeyECDSA, err := crypto.HexToECDSA(string(oraclePrivateKey[2:]))
	if err != nil {
		return nil, err
	}

	oraclePublicKey := oraclePrivateKeyECDSA.Public()
	log.Print("Public Key: ", hexutil.Encode(crypto.FromECDSAPub(oraclePublicKey.(*ecdsa.PublicKey))))

	ECDSAoraclePublicKey, err := crypto.UnmarshalPubkey(crypto.FromECDSAPub(oraclePublicKey.(*ecdsa.PublicKey)))
	if err != nil || ECDSAoraclePublicKey == nil {
		log.Print(err)
		log.Print(ECDSAoraclePublicKey)
		return nil, err
	}
	_, oracleAddress := walletworker.GenerateAddress(ECDSAoraclePublicKey)
	log.Print("Address: ", oracleAddress)

	transactOpts, err := bind.NewKeyedTransactorWithChainID(oraclePrivateKeyECDSA, chainID)
	if err != nil {
		return nil, err
	}

	gasPrice, err := client.SuggestGasPrice(context.Background())
	if err != nil {
		return nil, err
	}
	nonce, err := client.PendingNonceAt(context.Background(), common.HexToAddress(oracleAddress))
	if err != nil {
		return nil, err
	}
	transactOpts.Nonce = big.NewInt(int64(nonce))
	transactOpts.Value = big.NewInt(100)
	transactOpts.GasPrice = gasPrice
	transactOpts.GasLimit = uint64(100000) // in units
	transactOpts.Context = context.Background()

	return &VORD20Caller{
		client:           client,
		contractAddress:  contractAddress,
		instance:         instance,
		transactOpts:     transactOpts,
		callOpts:         &bind.CallOpts{},
		publicProvingKey: [2]*big.Int{ECDSAoraclePublicKey.X, ECDSAoraclePublicKey.Y},
		oraclePrivateKey: string(oraclePrivateKey),
		oraclePublicKey:  hexutil.Encode(crypto.FromECDSAPub(oraclePublicKey.(*ecdsa.PublicKey))),
		oracleAddress:    oracleAddress,
	}, err
}

func (d *VORD20Caller) RollDice(seed *big.Int) (*types.Transaction, error) {
	fmt.Println(d.transactOpts)
	fmt.Println(*seed)
	fmt.Println(common.HexToAddress(d.oracleAddress).Hex())
	return d.instance.RollDice(d.transactOpts, seed, common.HexToAddress(d.oracleAddress))
}

func (d *VORD20Caller) SetFee(fee *big.Int) (*types.Transaction, error) {
	return d.instance.SetFee(d.transactOpts, fee)
}
