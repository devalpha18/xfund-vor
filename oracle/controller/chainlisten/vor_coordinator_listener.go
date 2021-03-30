package chainlisten

import (
	"context"
	"fmt"
	"github.com/ethereum/go-ethereum"
	"github.com/ethereum/go-ethereum/accounts/abi"
	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/crypto"
	"github.com/ethereum/go-ethereum/ethclient"
	"math/big"
	"oracle/config"
	"oracle/contracts/vor_coordinator"
	"oracle/service"
	"oracle/tools/vor"
	"oracle/utils"
	"strings"
	"sync"
	"time"
)

type VORCoordinatorListener struct {
	contractAddress common.Address
	client          *ethclient.Client
	instance        *vor_coordinator.VORCoordinator
	query           ethereum.FilterQuery
	wg              *sync.WaitGroup
	service         *service.Service
	context         context.Context
}

func NewVORCoordinatorListener(contractHexAddress string, ethHostAddress string, service *service.Service, ctx context.Context) (*VORCoordinatorListener, error) {
	client, err := ethclient.Dial(ethHostAddress)
	if err != nil {
		return nil, err
	}
	contractAddress := common.HexToAddress(contractHexAddress)
	instance, err := vor_coordinator.NewVORCoordinator(contractAddress, client)
	if err != nil {
		return nil, err
	}

	var lastBlock *big.Int
	lastRequest, err := service.Store.RandomnessRequest.Last()
	if blockNumber, _ := service.Store.Keystorage.GetBlockNumber(); blockNumber != 0 {
		lastBlock = big.NewInt(blockNumber)
	} else if lastRequest != nil {
		lastBlock = big.NewInt(int64(lastRequest.GetBlockNumber()))
	} else if config.Conf.FirstBlockNumber != 0 {
		lastBlock = big.NewInt(int64(config.Conf.FirstBlockNumber))
	} else {
		lastBlock = big.NewInt(1)
	}

	return &VORCoordinatorListener{
		client:          client,
		contractAddress: contractAddress,
		instance:        instance,
		query: ethereum.FilterQuery{
			FromBlock: lastBlock,
			Addresses: []common.Address{contractAddress},
		},
		service: service,
		context: ctx,
		wg:      &sync.WaitGroup{},
	}, err
}

func (d VORCoordinatorListener) StartPoll() (err error) {
	d.wg.Add(1)
	for {
		err = d.Request()
		time.Sleep(3 * time.Minute)
	}
	d.wg.Wait()
	return
}

func (d *VORCoordinatorListener) SetLastBlockNumber(blockNumber uint64) (err error) {
	d.query.FromBlock = big.NewInt(int64(blockNumber))
	err = d.service.Store.Keystorage.SetBlockNumber(int64(blockNumber))
	return
}

func (d *VORCoordinatorListener) Request() error {
	logs, err := d.client.FilterLogs(context.Background(), d.query)
	if err != nil {
		return err
	}

	contractAbi, err := abi.JSON(strings.NewReader(string(vor_coordinator.VORCoordinatorABI)))
	if err != nil {
		return err
	}
	logRandomnessRequestSig := []byte("RandomnessRequest(bytes32,uint256,address,uint256,bytes32)")
	logRandomnessRequestHash := crypto.Keccak256Hash(logRandomnessRequestSig)

	fmt.Println("logRandomnessRequestHash hex: ", logRandomnessRequestHash.Hex())
	fmt.Println("logs: ", logs)

	for index, vLog := range logs {
		fmt.Println("----------------------------------------")
		fmt.Println("Log Block Number: ", vLog.BlockNumber)
		fmt.Println("Log Index: ", vLog.Index)
		if index == len(logs)-1 {
			err = d.SetLastBlockNumber(vLog.BlockNumber)
		}
		switch vLog.Topics[0].Hex() {
		case logRandomnessRequestHash.Hex():
			fmt.Println("Log Name: RandomnessRequest")

			//var randomnessRequestEvent contractModel.LogRandomnessRequest
			event := vor_coordinator.VORCoordinatorRandomnessRequest{}
			err := contractAbi.UnpackIntoInterface(&event, "RandomnessRequest", vLog.Data)
			if err != nil {
				return err
			}
			fmt.Println(common.Bytes2Hex(event.KeyHash[:]))
			fmt.Println(*event.Seed)
			fmt.Println(event.Sender.Hex())
			fmt.Println(*event.Fee)
			fmt.Println(common.Bytes2Hex(event.RequestID[:]))
			fmt.Println(vLog.BlockHash)
			fmt.Println(vLog.BlockNumber)
			fmt.Println(event.Raw)

			byteSeed, err := vor.BigToSeed(event.Seed)

			var status string
			tx, err := d.service.FulfillRandomness(byteSeed, vLog.BlockHash, int64(vLog.BlockNumber))
			fmt.Println(tx)
			if err != nil {
				status = "failed"
			} else {
				status = "success"
			}
			seedHex, err := utils.Uint256ToHex(event.Seed)
			err = d.service.Store.RandomnessRequest.Insert(common.Bytes2Hex(event.KeyHash[:]), seedHex, event.Sender.Hex(), common.Bytes2Hex(event.RequestID[:]), vLog.BlockHash.Hex(), vLog.BlockNumber, vLog.TxHash.Hex(), status)
			continue
		default:
			fmt.Println("vLog: ", vLog)
			continue
		}
	}

	return err
}

func (d VORCoordinatorListener) RandomnessRequest() {

}
