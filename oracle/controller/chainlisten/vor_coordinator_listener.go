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
	"math/rand"
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
	keyHash         [32]byte
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

	keyHash, err := service.VORCoordinatorCaller.HashOfKey()
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
		keyHash: keyHash,
		wg:      &sync.WaitGroup{},
	}, err
}

func (d VORCoordinatorListener) StartPoll() (err error) {
	d.wg.Add(1)
	var sleepTime = int32(3)
	if config.Conf.CheckDuration != 0 {
		sleepTime = config.Conf.CheckDuration
	}
	for {
		err = d.Request()
		time.Sleep(time.Duration(rand.Int31n(sleepTime)) * time.Second)
	}
	d.wg.Wait()
	return
}

func (d *VORCoordinatorListener) SetLastBlockNumber(blockNumber uint64) (err error) {
	d.query.FromBlock = big.NewInt(int64(blockNumber))
	err = d.service.Store.Keystorage.SetBlockNumber(int64(blockNumber + 1))
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
			if event.KeyHash == d.keyHash {

				fmt.Println("It's request to me =)")

				byteSeed, err := vor.BigToSeed(event.Seed)

				var status string
				tx, err := d.service.FulfillRandomness(byteSeed, vLog.BlockHash, int64(vLog.BlockNumber))
				fmt.Println(tx)
				if err == nil {
					fmt.Println(err)
					status = "failed"
				} else {
					status = "success"
				}
				seedHex, err := utils.Uint256ToHex(event.Seed)
				err = d.service.Store.RandomnessRequest.Insert(common.Bytes2Hex(event.KeyHash[:]), seedHex, event.Sender.Hex(), common.Bytes2Hex(event.RequestID[:]), vLog.BlockHash.Hex(), vLog.BlockNumber, vLog.TxHash.Hex(), status)
			} else {
				fmt.Println("Looks like it's addressed not to me =(")
			}
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
