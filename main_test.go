package main

import (
	"math/big"
	"testing"

	"github.com/ethereum/go-ethereum/core/types"
)

func testBlockReward(t *testing.T, block *types.Block, reward int64, mevReward int64, mev bool) {
	response := blockReward(block)
	if response.BlockReward.Cmp(big.NewInt(reward)) != 0 {
		t.Error("Incorrect block reward for block", block.Number(), "should be", reward, "not", response.BlockReward)
	}
	if response.MevReward.Cmp(big.NewInt(mevReward)) != 0 {
		t.Error("Incorrect mev reward for block", block.Number(), "should be", mevReward, "not", response.MevReward)
	}
	if response.Mev != mev {
		t.Error("MEV should be ", mev, "but is", response.Mev)
	}
}

func testRewardForSlot(t *testing.T, slot int, reward int64, mevReward int64, mev bool) {
	response, _, _ := rewardForSlot(slot)
	if response.BlockReward.Cmp(big.NewInt(reward)) != 0 {
		t.Error("Incorrect block reward for slot", slot, "should be", reward, "not", response.BlockReward)
	}
	if response.MevReward.Cmp(big.NewInt(mevReward)) != 0 {
		t.Error("Incorrect mev reward for slot", slot, "should be", mevReward, "not", response.MevReward)
	}
	if response.Mev != mev {
		t.Error("MEV should be", mev, "but is", response.Mev, "for slot", slot)
	}
}

func TestRewardForSlot(t *testing.T) {
	initApi()
	testRewardForSlot(t, 9197117, 113757939, 105971629, true)
	testRewardForSlot(t, 9197119, 49106970, 49426618, true)
	testRewardForSlot(t, 9197120, 4699116, 0, false)
	testRewardForSlot(t, 9197121, 47357205, 41136528, true)
	// skipped slot
	testRewardForSlot(t, 9208672, 0, 0, false)

	// Case where my logic falls apart since the miner has no transactions in the block, maybe such blocks are not MEV, even tough it was build by a mev-builder...
	// https://beaconcha.in/slot/9197117
	testRewardForSlot(t, 9197118, 18717163, 0, true) // MEV reward according to beaconcha.in is 0.10597 ETH
}

func TestBlockReward(t *testing.T) {
	initApi()
	testBlockReward(t, getBlockByNumber(19992375), 113757939, 105971629, true)
	testBlockReward(t, getBlockByNumber(19985006), 92597994, 94677617, true)
	testBlockReward(t, getBlockByNumber(19985005), 14665998, 0, false)
	testBlockReward(t, getBlockByNumber(19985007), 19866511, 70887761, true)
	testBlockReward(t, getBlockByNumber(19985021), 45081021, 44192994, true)
	testBlockReward(t, getBlockByNumber(15537400), 456279177, 0, false)
	testBlockReward(t, getBlockByNumber(15537300), 2117027595, 0, false)
	testBlockReward(t, getBlockByNumber(7280900), 2262921137, 0, false)
	testBlockReward(t, getBlockByNumber(7270000), 3106084374, 0, false)
	testBlockReward(t, getBlockByNumber(7270000), 3106084374, 0, false)
	testBlockReward(t, getBlockByNumber(4370100), 3111209062, 0, false)
	testBlockReward(t, getBlockByNumber(4360100), 5181177404, 0, false)
}
