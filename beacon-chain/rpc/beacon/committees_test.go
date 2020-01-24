package beacon

import (
	"context"
	"encoding/binary"
	"reflect"
	"testing"

	"github.com/gogo/protobuf/proto"
	ethpb "github.com/prysmaticlabs/ethereumapis/eth/v1alpha1"
	mock "github.com/prysmaticlabs/prysm/beacon-chain/blockchain/testing"
	"github.com/prysmaticlabs/prysm/beacon-chain/core/helpers"
	"github.com/prysmaticlabs/prysm/beacon-chain/db"
	dbTest "github.com/prysmaticlabs/prysm/beacon-chain/db/testing"
	stateTrie "github.com/prysmaticlabs/prysm/beacon-chain/state"
	pbp2p "github.com/prysmaticlabs/prysm/proto/beacon/p2p/v1"
	"gopkg.in/d4l3k/messagediff.v1"

	"github.com/prysmaticlabs/prysm/shared/params"
)

func TestServer_ListBeaconCommittees(t *testing.T) {
	db := dbTest.SetupDB(t)
	defer dbTest.TeardownDB(t, db)

	numValidators := 128
	headState := setupActiveValidators(t, db, numValidators)

	randaoMixes := make([][]byte, params.BeaconConfig().EpochsPerHistoricalVector)
	for i := 0; i < len(randaoMixes); i++ {
		randaoMixes[i] = make([]byte, 32)
	}
	if err := headState.SetRandaoMixes(randaoMixes); err != nil {
		t.Fatal(err)
	}

	bs := &Server{
		HeadFetcher: &mock.ChainService{
			State: headState,
		},
	}

	activeIndices, err := helpers.ActiveValidatorIndices(headState, 0)
	if err != nil {
		t.Fatal(err)
	}
	attesterSeed, err := helpers.Seed(headState, 0, params.BeaconConfig().DomainBeaconAttester)
	if err != nil {
		t.Fatal(err)
	}
	wanted := make(map[uint64]*ethpb.BeaconCommittees_CommitteesList)
	for slot := uint64(0); slot < params.BeaconConfig().SlotsPerEpoch; slot++ {
		var countAtSlot = uint64(numValidators) / params.BeaconConfig().SlotsPerEpoch / params.BeaconConfig().TargetCommitteeSize
		if countAtSlot > params.BeaconConfig().MaxCommitteesPerSlot {
			countAtSlot = params.BeaconConfig().MaxCommitteesPerSlot
		}
		if countAtSlot == 0 {
			countAtSlot = 1
		}
		committeeItems := make([]*ethpb.BeaconCommittees_CommitteeItem, countAtSlot)
		for i := uint64(0); i < countAtSlot; i++ {
			epochOffset := i + (slot%params.BeaconConfig().SlotsPerEpoch)*countAtSlot
			totalCount := countAtSlot * params.BeaconConfig().SlotsPerEpoch
			committee, err := helpers.ComputeCommittee(activeIndices, attesterSeed, epochOffset, totalCount)
			if err != nil {
				t.Fatal(err)
			}
			committeeItems[i] = &ethpb.BeaconCommittees_CommitteeItem{
				ValidatorIndices: committee,
			}
		}
		wanted[slot] = &ethpb.BeaconCommittees_CommitteesList{
			Committees: committeeItems,
		}
	}

	tests := []struct {
		req *ethpb.ListCommitteesRequest
		res *ethpb.BeaconCommittees
	}{
		{
			req: &ethpb.ListCommitteesRequest{},
			res: &ethpb.BeaconCommittees{
				Epoch:                0,
				Committees:           wanted,
				ActiveValidatorCount: uint64(numValidators),
			},
		},
	}
	for _, test := range tests {
		res, err := bs.ListBeaconCommittees(context.Background(), test.req)
		if err != nil {
			t.Fatal(err)
		}
		if !proto.Equal(res, test.res) {
			t.Errorf("Expected %v, received %v", test.res, res)
		}
	}
}

func TestServer_ListBeaconCommittees_FromArchive(t *testing.T) {
	db := dbTest.SetupDB(t)
	defer dbTest.TeardownDB(t, db)
	ctx := context.Background()

	numValidators := 128
	headState := setupActiveValidators(t, db, numValidators)

	randaoMixes := make([][]byte, params.BeaconConfig().EpochsPerHistoricalVector)
	for i := 0; i < len(randaoMixes); i++ {
		randaoMixes[i] = make([]byte, 32)
	}
	if err := headState.SetRandaoMixes(randaoMixes); err != nil {
		t.Fatal(err)
	}

	if err := headState.SetSlot(params.BeaconConfig().SlotsPerEpoch * 10); err != nil {
		t.Fatal(err)
	}

	// Store the genesis seed.
	seed, err := helpers.Seed(headState, 0, params.BeaconConfig().DomainBeaconAttester)
	if err != nil {
		t.Fatal(err)
	}
	if err := db.SaveArchivedCommitteeInfo(ctx, 0, &pbp2p.ArchivedCommitteeInfo{
		AttesterSeed: seed[:],
	}); err != nil {
		t.Fatal(err)
	}

	bs := &Server{
		BeaconDB: db,
		HeadFetcher: &mock.ChainService{
			State: headState,
		},
	}

	activeIndices, err := helpers.ActiveValidatorIndices(headState, 0)
	if err != nil {
		t.Fatal(err)
	}
	wanted := make(map[uint64]*ethpb.BeaconCommittees_CommitteesList)
	for slot := uint64(0); slot < params.BeaconConfig().SlotsPerEpoch; slot++ {
		var countAtSlot = uint64(numValidators) / params.BeaconConfig().SlotsPerEpoch / params.BeaconConfig().TargetCommitteeSize
		if countAtSlot > params.BeaconConfig().MaxCommitteesPerSlot {
			countAtSlot = params.BeaconConfig().MaxCommitteesPerSlot
		}
		if countAtSlot == 0 {
			countAtSlot = 1
		}
		committeeItems := make([]*ethpb.BeaconCommittees_CommitteeItem, countAtSlot)
		for i := uint64(0); i < countAtSlot; i++ {
			epochOffset := i + (slot%params.BeaconConfig().SlotsPerEpoch)*countAtSlot
			totalCount := countAtSlot * params.BeaconConfig().SlotsPerEpoch
			committee, err := helpers.ComputeCommittee(activeIndices, seed, epochOffset, totalCount)
			if err != nil {
				t.Fatal(err)
			}
			committeeItems[i] = &ethpb.BeaconCommittees_CommitteeItem{
				ValidatorIndices: committee,
			}
		}
		wanted[slot] = &ethpb.BeaconCommittees_CommitteesList{
			Committees: committeeItems,
		}
	}
	res1, err := bs.ListBeaconCommittees(context.Background(), &ethpb.ListCommitteesRequest{
		QueryFilter: &ethpb.ListCommitteesRequest_Genesis{
			Genesis: true,
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	res2, err := bs.ListBeaconCommittees(context.Background(), &ethpb.ListCommitteesRequest{
		QueryFilter: &ethpb.ListCommitteesRequest_Epoch{
			Epoch: 0,
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(res1, res2) {
		t.Fatal(err)
	}
	wantedRes := &ethpb.BeaconCommittees{
		Epoch:                0,
		Committees:           wanted,
		ActiveValidatorCount: uint64(numValidators),
	}
	if !reflect.DeepEqual(wantedRes, res1) {
		t.Error(messagediff.PrettyDiff(wantedRes, res1))
	}
}

func setupActiveValidators(t *testing.T, db db.Database, count int) *stateTrie.BeaconState {
	ctx := context.Background()
	balances := make([]uint64, count)
	validators := make([]*ethpb.Validator, 0, count)
	for i := 0; i < count; i++ {
		pubKey := make([]byte, params.BeaconConfig().BLSPubkeyLength)
		binary.LittleEndian.PutUint64(pubKey, uint64(i))
		if err := db.SaveValidatorIndex(ctx, pubKey, uint64(i)); err != nil {
			t.Fatal(err)
		}
		balances[i] = uint64(i)
		validators = append(validators, &ethpb.Validator{
			PublicKey:             pubKey,
			ActivationEpoch:       0,
			ExitEpoch:             params.BeaconConfig().FarFutureEpoch,
			WithdrawalCredentials: make([]byte, 32),
		})
	}
	st, err := stateTrie.InitializeFromProto(&pbp2p.BeaconState{Validators: validators, Balances: balances})
	if err != nil {
		t.Fatal(err)
	}
	return st
}
