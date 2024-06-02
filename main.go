package main

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"math/big"
	"net/http"
	"os"
	"strconv"
	"strings"
	"sync"
	"time"

	mapset "github.com/deckarep/golang-set/v2"
	docs "github.com/durch/eth-validator-api/docs"
	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/core"
	"github.com/ethereum/go-ethereum/core/types"
	"github.com/ethereum/go-ethereum/ethclient"
	"github.com/gin-gonic/gin"
	"github.com/go-deeper/chunks"
	swaggerfiles "github.com/swaggo/files"
	ginSwagger "github.com/swaggo/gin-swagger"
)

const (
	byzantiumBlockNumber      = 4370000
	constantinopleBlockNumber = 7280000
	theMergeBlockNumber       = 15537392
	apiUrl                    = "https://sparkling-boldest-bridge.quiknode.pro/446746cc467542de08437e2eb9908ed70f838f1d/"
)

func getEthClient() (*ethclient.Client, error) {
	client, err := ethclient.Dial(apiUrl)
	if err != nil {
		log.Fatal(err)
	}
	return client, err
}

// GetSyncCommittee godoc
// @Summary Get validators with sync committee duties for a slot
// @Schemes
// @Description Get validators with sync committee duties for a slot
// @Param slot path int true "Slot"
// @Produce json
// @Success 200 {json} ethrpc.Block
// @Router /syncduties/{slot} [get]
func GetSyncCommittee(g *gin.Context) {
	param := g.Param("slot")
	slot, err := strconv.Atoi(param)
	if err != nil {
		g.AbortWithStatusJSON(http.StatusBadRequest, gin.H{"msg": "Invalid slot"})
		return
	}

	headSlot, err := getHeadSlot()

	if err != nil {
		g.AbortWithStatusJSON(http.StatusInternalServerError, gin.H{"msg": "Could not get head slot"})
		return
	}

	headSlotNumber, err := strconv.Atoi(headSlot["data"].(map[string]interface{})["message"].(map[string]interface{})["slot"].(string))
	if err != nil {
		g.AbortWithStatusJSON(http.StatusInternalServerError, gin.H{"msg": "Could not get valid head slot"})
		return
	}

	syncCommitteeCh := make(chan mapset.Set[string])
	validatorsCh := make(chan map[string]interface{})
	go func() {
		syncCommitteeCh <- getSyncCommitteeForSlot(slot)
	}()

	go func() {
		validatorsCh <- getValidators(slot)
	}()

	syncCommittee := <-syncCommitteeCh
	validators := <-validatorsCh

	if syncCommittee.Cardinality() == 0 && slot > headSlotNumber {
		g.AbortWithStatusJSON(http.StatusBadRequest, gin.H{"msg": "Slot is to far in the future"})
	}

	if validators == nil {
		g.AbortWithStatusJSON(http.StatusInternalServerError, gin.H{"msg": "Could not get validators for slot"})
		return
	}

	validatorList := validators["data"].([]interface{})

	pubkeys := make([]string, 0, syncCommittee.Cardinality())

	for _, validator := range validatorList {
		v := validator.(map[string]interface{})
		if syncCommittee.Contains(v["index"].(string)) {
			pubkeys = append(pubkeys, v["validator"].(map[string]interface{})["pubkey"].(string))
		}
	}

	g.JSON(http.StatusOK, pubkeys)
}

func getSlot(slot int) (map[string]interface{}, error) {
	url := fmt.Sprintf("%seth/v2/beacon/blocks/%d", apiUrl, slot)
	resp, err := http.Get(url)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	var jResp map[string]interface{}

	if err := json.NewDecoder(resp.Body).Decode(&jResp); err != nil {
		return nil, err
	}
	return jResp, nil
}

func getHeadSlot() (map[string]interface{}, error) {
	url := fmt.Sprintf("%seth/v2/beacon/blocks/head", apiUrl)
	resp, err := http.Get(url)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	var jResp map[string]interface{}

	if err := json.NewDecoder(resp.Body).Decode(&jResp); err != nil {
		return nil, err
	}
	return jResp, nil
}

func getSyncCommitteeForSlot(slot int) mapset.Set[string] {
	val, ok := syncCommitteeCache.Get(slot)
	if ok {
		return val
	}
	url := fmt.Sprintf("%seth/v1/beacon/states/%d/sync_committees", apiUrl, slot)

	resp, err := http.Get(url)
	if err != nil {
		log.Fatalln(err)
		return mapset.NewSet[string]()
	}
	defer resp.Body.Close()

	var jResp2 map[string]interface{}

	if err := json.NewDecoder(resp.Body).Decode(&jResp2); err != nil {
		log.Fatal(err)
		return mapset.NewSet[string]()
	}

	code, ok := jResp2["code"]
	if ok {
		if code.(float64) == 404 {
			return mapset.NewSet[string]()
		}
	}

	validators := jResp2["data"].(map[string]interface{})["validators"].([]interface{})
	validatorSet := mapset.NewSet[string]()
	for _, validator := range validators {
		validatorSet.Add(string(validator.(string)))
	}

	syncCommitteeCache.Set(slot, validatorSet)

	return validatorSet
}

func getBlockForSlot(slot int) (string, error, bool) {
	jResp, err := getSlot(slot)
	if err != nil {
		return "", err, false
	}

	code, ok := jResp["code"]
	if ok {
		if code.(float64) == 404 {
			return "", nil, true
		}
	}

	return jResp["data"].(map[string]interface{})["message"].(map[string]interface{})["body"].(map[string]interface{})["execution_payload"].(map[string]interface{})["block_hash"].(string), nil, false
}

// @BasePath /

// BlockReward godoc
// @Summary Get blockreward for slot
// @Schemes
// @Description Get block and mev reward and mev status for slot
// @Param slot path int true "Slot"
// @Produce json
// @Success 200 {json} BlockRewardResponse
// @Router /blockreward/{slot} [get]
func BlockReward(g *gin.Context) {
	param := g.Param("slot")
	slot, err := strconv.Atoi(param)
	if err != nil {
		g.AbortWithStatusJSON(http.StatusBadRequest, gin.H{"msg": "Invalid slot"})
		return
	}

	headSlot, err := getHeadSlot()

	if err != nil {
		g.AbortWithStatusJSON(http.StatusInternalServerError, gin.H{"msg": "Could not get head slot"})
		return
	}

	headSlotNumber, err := strconv.Atoi(headSlot["data"].(map[string]interface{})["message"].(map[string]interface{})["slot"].(string))
	if err != nil {
		g.AbortWithStatusJSON(http.StatusInternalServerError, gin.H{"msg": "Could not get valid head slot"})
		return
	}

	if slot > headSlotNumber {
		g.AbortWithStatusJSON(http.StatusBadRequest, gin.H{"msg": "Slot is in the future"})
		return
	}

	response, err, notFound := rewardForSlot(slot)
	if err != nil {
		g.AbortWithStatusJSON(http.StatusInternalServerError, gin.H{"msg": "Could not get reward for slot"})
		return
	}

	if notFound {
		g.AbortWithStatusJSON(http.StatusNotFound, gin.H{"msg": "Slot does not exist or was skipped"})
		return
	}

	g.JSON(http.StatusOK, response)
}

func rewardForSlot(slot int) (BlockRewardResponse, error, bool) {
	blockHash, err, notFound := getBlockForSlot(slot)
	if err != nil {
		return BlockRewardResponse{
			Mev:         false,
			BlockReward: big.NewInt(0),
			MevReward:   big.NewInt(0),
		}, err, false
	}
	if notFound {
		return BlockRewardResponse{
			Mev:         false,
			BlockReward: big.NewInt(0),
			MevReward:   big.NewInt(0),
		}, nil, true
	}
	block, err := getBlockByHash(blockHash)
	if err != nil {
		return BlockRewardResponse{
			Mev:         false,
			BlockReward: big.NewInt(0),
			MevReward:   big.NewInt(0),
		}, err, false
	}

	response := blockReward(block)
	return response, nil, false
}

type BlockRewardResponse struct {
	Mev         bool     `json:"status"`
	BlockReward *big.Int `json:"blockReward"`
	MevReward   *big.Int `json:"mevReward"`
}

func findMevRewardTransaction(block *types.Block) *big.Int {
	value := big.NewInt(0)
	for _, tx := range block.Transactions() {
		msg, err := core.TransactionToMessage(tx, types.LatestSignerForChainID(tx.ChainId()), nil)
		if err != nil {
			panic(err)
		}
		// There can be more then one (seen two) transaction from the builder address, lets use the biggest one, that seems to be what the explorers are doing
		// example https://beaconcha.in/block/19992375#overview
		if msg.From == block.Coinbase() {
			if value.Cmp(tx.Value()) == -1 {
				value = tx.Value()
			}
			// log.Println(tx.Hash(), tx.To())
		}
	}
	// log.Println(mevTransactions[0])
	return value
}

func blockReward(block *types.Block) BlockRewardResponse {
	mev := isMev(block)
	burntFees := calculateBurntFees(block)
	mevReward := big.NewInt(0)
	if mev {
		mevReward = findMevRewardTransaction(block)
	}
	staticBlockReward := big.NewInt(int64(calculateBlockReward(int(block.Number().Int64()))))
	transactionFees := transactionFees(block)
	if Verbose {
		log.Println(staticBlockReward, transactionFees, burntFees)
	}

	transactionFees.Sub(transactionFees, burntFees)
	transactionFees.Add(transactionFees, staticBlockReward)

	response := BlockRewardResponse{
		Mev:         mev,
		BlockReward: transactionFees.Div(transactionFees, big.NewInt(1e9)),
		MevReward:   mevReward.Div(mevReward, big.NewInt(1e9)),
	}

	return response
}

func calculateBurntFees(block *types.Block) *big.Int {
	baseFee := block.BaseFee()
	if baseFee == nil {
		return big.NewInt(0)
	}
	return big.NewInt(0).Mul(baseFee, big.NewInt(int64(block.GasUsed())))

}

func calculateBlockReward(blockNumber int) int {
	switch {
	case blockNumber < byzantiumBlockNumber && blockNumber > 0:
		return 5000000000000000000
	case blockNumber >= byzantiumBlockNumber && blockNumber < constantinopleBlockNumber:
		return 3000000000000000000
	case blockNumber >= constantinopleBlockNumber && blockNumber <= theMergeBlockNumber:
		return 2000000000000000000
	case blockNumber > theMergeBlockNumber:
		return 0
	default:
		return 0
	}
}

func getTransactionReceipt(tx *types.Transaction) *types.Receipt {
	val, ok := receiptCache.Get(tx.Hash())
	if ok {
		return val
	} else {
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		receipt, err := Client.TransactionReceipt(ctx, tx.Hash())
		if err != nil {
			log.Println(err)
		}
		receiptCache.Set(tx.Hash(), receipt)
		return receipt
	}

}

func transactionFee(tx *types.Transaction, chn chan *big.Int, verbose bool) {
	receipt := getTransactionReceipt(tx)
	gas := big.NewInt(int64(receipt.GasUsed))
	tx_fee := big.NewInt(0).Mul(receipt.EffectiveGasPrice, gas)
	if verbose {
		log.Println(tx.Hash(), tx.GasPrice(), gas, tx_fee)
	}
	chn <- tx_fee
}

func getValidators(slot int) map[string]interface{} {
	val, ok := validatorsCache.Get(slot)
	if ok {
		return val
	}
	url := fmt.Sprintf("%seth/v1/beacon/states/%d/validators/", apiUrl, slot)
	resp, err := http.Get(url)
	if err != nil {
		log.Fatalln(err)
		return nil
	}
	defer resp.Body.Close()

	var jResp map[string]interface{}

	if err := json.NewDecoder(resp.Body).Decode(&jResp); err != nil {
		log.Fatal(err)
		return nil
	}

	validatorsCache.Set(slot, jResp)
	return jResp

}

func transactionFees(block *types.Block) *big.Int {
	var wg sync.WaitGroup

	sum := big.NewInt(0)
	chn := make(chan *big.Int, block.Transactions().Len())

	slices := chunks.Split(block.Transactions(), 10)

	for _, slice := range slices {
		for _, tx := range slice {
			wg.Add(1)
			go func(t *types.Transaction) {
				defer wg.Done()
				transactionFee(t, chn, Verbose)
			}(tx)
		}
		wg.Wait()
	}

	close(chn)

	for s := range chn {
		sum.Add(sum, s)
	}

	return sum
}

func isMev(block *types.Block) bool {
	fee_recipient := strings.ToLower(block.Coinbase().Hex())
	if _, ok := mev_builders[fee_recipient]; ok {
		return true
	}
	return false
}

func getBlockByNumber(blockNumber int) *types.Block {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	block, err := Client.BlockByNumber(ctx, big.NewInt(int64(blockNumber)))
	if err != nil {
		log.Fatal(err)
		return nil
	}

	return block
}

func getBlockByHash(blockHash string) (*types.Block, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	hash := common.HexToHash(blockHash)
	block, err := Client.BlockByHash(ctx, hash)
	if err != nil {
		return nil, err
	}

	return block, nil
}

func getMevBuilders() {
	mevFile, err := os.Open("mev.json")
	if err != nil {
		panic(err)
	}

	jsonParser := json.NewDecoder(mevFile)
	if err = jsonParser.Decode(&mev_builders); err != nil {
		panic(err)
	}
}

type CommitteeCache struct {
	cache map[int]mapset.Set[string]
	mtx   sync.RWMutex
}

func (cc *CommitteeCache) Get(key int) (mapset.Set[string], bool) {
	cc.mtx.RLock()
	defer cc.mtx.RUnlock()
	val, ok := cc.cache[key]
	return val, ok
}

func (cc *CommitteeCache) Set(key int, value mapset.Set[string]) {
	cc.mtx.Lock()
	defer cc.mtx.Unlock()
	cc.cache[key] = value
}

type ValidatorsCache struct {
	cache map[int]map[string]interface{}
	mtx   sync.RWMutex
}

func (vc *ValidatorsCache) Get(key int) (map[string]interface{}, bool) {
	vc.mtx.RLock()
	defer vc.mtx.RUnlock()
	val, ok := vc.cache[key]
	return val, ok
}

func (vc *ValidatorsCache) Set(key int, value map[string]interface{}) {
	vc.mtx.Lock()
	defer vc.mtx.Unlock()
	vc.cache[key] = value
}

type ReceiptCache struct {
	cache map[common.Hash]*types.Receipt
	mtx   sync.RWMutex
}

func (rc *ReceiptCache) Get(key common.Hash) (*types.Receipt, bool) {
	rc.mtx.RLock()
	defer rc.mtx.RUnlock()
	val, ok := rc.cache[key]
	return val, ok
}

func (rc *ReceiptCache) Set(key common.Hash, value *types.Receipt) {
	rc.mtx.Lock()
	defer rc.mtx.Unlock()
	rc.cache[key] = value
}

var mev_builders map[string]string
var Client *ethclient.Client
var Verbose bool = false
var receiptCache ReceiptCache
var syncCommitteeCache CommitteeCache
var validatorsCache ValidatorsCache

func initApi() {
	client, err := getEthClient()
	if err != nil {
		panic(err)
	}
	getMevBuilders()
	receiptCache = ReceiptCache{make(map[common.Hash]*types.Receipt), sync.RWMutex{}}
	syncCommitteeCache = CommitteeCache{make(map[int]mapset.Set[string]), sync.RWMutex{}}
	validatorsCache = ValidatorsCache{make(map[int]map[string]interface{}), sync.RWMutex{}}
	Client = client
}

func main() {
	initApi()
	r := gin.Default()
	docs.SwaggerInfo.BasePath = "/"
	v1 := r.Group("/")
	{
		v1.GET("/blockreward/:slot", BlockReward)
		v1.GET("/syncduties/:slot", GetSyncCommittee)
	}
	r.GET("/swagger/*any", ginSwagger.WrapHandler(swaggerfiles.Handler))
	r.Run(":8080")

}
