package lightclient

import (
	"context"
	"fmt"
	"math"
	"net/http"

	"github.com/ethereum/go-ethereum/common/hexutil"
	"github.com/pkg/errors"
	"github.com/prysmaticlabs/prysm/v5/api"
	"github.com/prysmaticlabs/prysm/v5/api/server/structs"
	lightclient "github.com/prysmaticlabs/prysm/v5/beacon-chain/core/light-client"
	"github.com/prysmaticlabs/prysm/v5/beacon-chain/rpc/eth/shared"
	"github.com/prysmaticlabs/prysm/v5/config/features"
	"github.com/prysmaticlabs/prysm/v5/config/params"
	"github.com/prysmaticlabs/prysm/v5/consensus-types/interfaces"
	"github.com/prysmaticlabs/prysm/v5/monitoring/tracing/trace"
	"github.com/prysmaticlabs/prysm/v5/network/httputil"
	"github.com/prysmaticlabs/prysm/v5/runtime/version"
	"github.com/wealdtech/go-bytesutil"
)

// GetLightClientBootstrap - implements https://github.com/ethereum/beacon-APIs/blob/263f4ed6c263c967f13279c7a9f5629b51c5fc55/apis/beacon/light_client/bootstrap.yaml
func (s *Server) GetLightClientBootstrap(w http.ResponseWriter, req *http.Request) {
	if !features.Get().EnableLightClient {
		httputil.HandleError(w, "Light client feature flag is not enabled", http.StatusNotFound)
		return
	}

	// Prepare
	ctx, span := trace.StartSpan(req.Context(), "beacon.GetLightClientBootstrap")
	defer span.End()

	// Get the block
	blockRootParam, err := hexutil.Decode(req.PathValue("block_root"))
	if err != nil {
		httputil.HandleError(w, "invalid block root: "+err.Error(), http.StatusBadRequest)
		return
	}

	blockRoot := bytesutil.ToBytes32(blockRootParam)
	blk, err := s.Blocker.Block(ctx, blockRoot[:])
	if !shared.WriteBlockFetchError(w, blk, err) {
		return
	}

	// Get the state
	state, err := s.Stater.StateBySlot(ctx, blk.Block().Slot())
	if err != nil {
		httputil.HandleError(w, "could not get state: "+err.Error(), http.StatusInternalServerError)
		return
	}

	bootstrap, err := lightclient.NewLightClientBootstrapFromBeaconState(ctx, s.ChainInfoFetcher.CurrentSlot(), state, blk)
	if err != nil {
		httputil.HandleError(w, "could not get light client bootstrap: "+err.Error(), http.StatusInternalServerError)
		return
	}
	w.Header().Set(api.VersionHeader, version.String(bootstrap.Version()))

	if httputil.RespondWithSsz(req) {
		ssz, err := bootstrap.MarshalSSZ()
		if err != nil {
			httputil.HandleError(w, "could not marshal bootstrap to SSZ: "+err.Error(), http.StatusInternalServerError)
			return
		}
		httputil.WriteSsz(w, ssz, "light_client_bootstrap.ssz")
	} else {
		data, err := structs.LightClientBootstrapFromConsensus(bootstrap)
		if err != nil {
			httputil.HandleError(w, "could not marshal bootstrap to JSON: "+err.Error(), http.StatusInternalServerError)
			return
		}
		response := &structs.LightClientBootstrapResponse{
			Version: version.String(bootstrap.Version()),
			Data:    data,
		}
		httputil.WriteJson(w, response)
	}
}

// GetLightClientUpdatesByRange - implements https://github.com/ethereum/beacon-APIs/blob/263f4ed6c263c967f13279c7a9f5629b51c5fc55/apis/beacon/light_client/updates.yaml
func (s *Server) GetLightClientUpdatesByRange(w http.ResponseWriter, req *http.Request) {
	if !features.Get().EnableLightClient {
		httputil.HandleError(w, "Light client feature flag is not enabled", http.StatusNotFound)
		return
	}

	ctx, span := trace.StartSpan(req.Context(), "beacon.GetLightClientUpdatesByRange")
	defer span.End()

	config := params.BeaconConfig()

	_, count, gotCount := shared.UintFromQuery(w, req, "count", true)
	if !gotCount {
		return
	} else if count == 0 {
		httputil.HandleError(w, fmt.Sprintf("Got invalid 'count' query variable '%d': count must be greater than 0", count), http.StatusBadRequest)
		return
	}

	if count > config.MaxRequestLightClientUpdates {
		count = config.MaxRequestLightClientUpdates
	}

	_, startPeriod, gotStartPeriod := shared.UintFromQuery(w, req, "start_period", true)
	if !gotStartPeriod {
		return
	}

	endPeriod := startPeriod + count - 1

	// get updates
	updatesMap, err := s.BeaconDB.LightClientUpdates(ctx, startPeriod, endPeriod)
	if err != nil {
		httputil.HandleError(w, "Could not get light client updates from DB: "+err.Error(), http.StatusInternalServerError)
		return
	}

	updates := make([]*structs.LightClientUpdateResponse, 0, len(updatesMap))

	for i := startPeriod; i <= endPeriod; i++ {
		update, ok := updatesMap[i]
		if !ok {
			// Only return the first contiguous range of updates
			break
		}

		updateJson, err := structs.LightClientUpdateFromConsensus(update)
		if err != nil {
			httputil.HandleError(w, "Could not convert light client update: "+err.Error(), http.StatusInternalServerError)
			return
		}
		updateResponse := &structs.LightClientUpdateResponse{
			Version: version.String(update.Version()),
			Data:    updateJson,
		}
		updates = append(updates, updateResponse)
	}
	httputil.WriteJson(w, updates)
}

// GetLightClientFinalityUpdate - implements https://github.com/ethereum/beacon-APIs/blob/263f4ed6c263c967f13279c7a9f5629b51c5fc55/apis/beacon/light_client/finality_update.yaml
func (s *Server) GetLightClientFinalityUpdate(w http.ResponseWriter, req *http.Request) {
	if !features.Get().EnableLightClient {
		httputil.HandleError(w, "Light client feature flag is not enabled", http.StatusNotFound)
		return
	}

	ctx, span := trace.StartSpan(req.Context(), "beacon.GetLightClientFinalityUpdate")
	defer span.End()

	// Finality update needs super majority of sync committee signatures
	minSyncCommitteeParticipants := float64(params.BeaconConfig().MinSyncCommitteeParticipants)
	minSignatures := uint64(math.Ceil(minSyncCommitteeParticipants * 2 / 3))

	block, err := s.suitableBlock(ctx, minSignatures)
	if !shared.WriteBlockFetchError(w, block, err) {
		return
	}

	st, err := s.Stater.StateBySlot(ctx, block.Block().Slot())
	if err != nil {
		httputil.HandleError(w, "Could not get state: "+err.Error(), http.StatusInternalServerError)
		return
	}

	attestedRoot := block.Block().ParentRoot()
	attestedBlock, err := s.Blocker.Block(ctx, attestedRoot[:])
	if !shared.WriteBlockFetchError(w, block, errors.Wrap(err, "could not get attested block")) {
		return
	}
	attestedSlot := attestedBlock.Block().Slot()
	attestedState, err := s.Stater.StateBySlot(ctx, attestedSlot)
	if err != nil {
		httputil.HandleError(w, "Could not get attested state: "+err.Error(), http.StatusInternalServerError)
		return
	}

	var finalizedBlock interfaces.ReadOnlySignedBeaconBlock
	finalizedCheckpoint := attestedState.FinalizedCheckpoint()
	if finalizedCheckpoint == nil {
		httputil.HandleError(w, "Attested state does not have a finalized checkpoint", http.StatusInternalServerError)
		return
	}
	finalizedRoot := bytesutil.ToBytes32(finalizedCheckpoint.Root)
	finalizedBlock, err = s.Blocker.Block(ctx, finalizedRoot[:])
	if !shared.WriteBlockFetchError(w, block, errors.Wrap(err, "could not get finalized block")) {
		return
	}

	update, err := newLightClientFinalityUpdateFromBeaconState(ctx, s.ChainInfoFetcher.CurrentSlot(), st, block, attestedState, attestedBlock, finalizedBlock)
	if err != nil {
		httputil.HandleError(w, "Could not get light client finality update: "+err.Error(), http.StatusInternalServerError)
		return
	}

	response := &structs.LightClientFinalityUpdateResponse{
		Version: version.String(attestedState.Version()),
		Data:    update,
	}

	httputil.WriteJson(w, response)
}

// GetLightClientOptimisticUpdate - implements https://github.com/ethereum/beacon-APIs/blob/263f4ed6c263c967f13279c7a9f5629b51c5fc55/apis/beacon/light_client/optimistic_update.yaml
func (s *Server) GetLightClientOptimisticUpdate(w http.ResponseWriter, req *http.Request) {
	if !features.Get().EnableLightClient {
		httputil.HandleError(w, "Light client feature flag is not enabled", http.StatusNotFound)
		return
	}

	ctx, span := trace.StartSpan(req.Context(), "beacon.GetLightClientOptimisticUpdate")
	defer span.End()

	block, err := s.suitableBlock(ctx, params.BeaconConfig().MinSyncCommitteeParticipants)
	if !shared.WriteBlockFetchError(w, block, err) {
		return
	}
	st, err := s.Stater.StateBySlot(ctx, block.Block().Slot())
	if err != nil {
		httputil.HandleError(w, "could not get state: "+err.Error(), http.StatusInternalServerError)
		return
	}
	attestedRoot := block.Block().ParentRoot()
	attestedBlock, err := s.Blocker.Block(ctx, attestedRoot[:])
	if err != nil {
		httputil.HandleError(w, "Could not get attested block: "+err.Error(), http.StatusInternalServerError)
		return
	}
	if attestedBlock == nil {
		httputil.HandleError(w, "Attested block is nil", http.StatusInternalServerError)
		return
	}
	attestedSlot := attestedBlock.Block().Slot()
	attestedState, err := s.Stater.StateBySlot(ctx, attestedSlot)
	if err != nil {
		httputil.HandleError(w, "Could not get attested state: "+err.Error(), http.StatusInternalServerError)
		return
	}

	update, err := newLightClientOptimisticUpdateFromBeaconState(ctx, s.ChainInfoFetcher.CurrentSlot(), st, block, attestedState, attestedBlock)
	if err != nil {
		httputil.HandleError(w, "Could not get light client optimistic update: "+err.Error(), http.StatusInternalServerError)
		return
	}

	response := &structs.LightClientOptimisticUpdateResponse{
		Version: version.String(attestedState.Version()),
		Data:    update,
	}

	httputil.WriteJson(w, response)
}

// suitableBlock returns the latest block that satisfies all criteria required for creating a new update
func (s *Server) suitableBlock(ctx context.Context, minSignaturesRequired uint64) (interfaces.ReadOnlySignedBeaconBlock, error) {
	st, err := s.HeadFetcher.HeadState(ctx)
	if err != nil {
		return nil, errors.Wrap(err, "could not get head state")
	}

	latestBlockHeader := st.LatestBlockHeader()
	stateRoot, err := st.HashTreeRoot(ctx)
	if err != nil {
		return nil, errors.Wrap(err, "could not get state root")
	}
	latestBlockHeader.StateRoot = stateRoot[:]
	latestBlockHeaderRoot, err := latestBlockHeader.HashTreeRoot()
	if err != nil {
		return nil, errors.Wrap(err, "could not get latest block header root")
	}

	block, err := s.Blocker.Block(ctx, latestBlockHeaderRoot[:])
	if err != nil {
		return nil, errors.Wrap(err, "could not get latest block")
	}
	if block == nil {
		return nil, errors.New("latest block is nil")
	}

	// Loop through the blocks until we find a block that satisfies minSignaturesRequired requirement
	var numOfSyncCommitteeSignatures uint64
	if syncAggregate, err := block.Block().Body().SyncAggregate(); err == nil {
		numOfSyncCommitteeSignatures = syncAggregate.SyncCommitteeBits.Count()
	}

	for numOfSyncCommitteeSignatures < minSignaturesRequired {
		// Get the parent block
		parentRoot := block.Block().ParentRoot()
		block, err = s.Blocker.Block(ctx, parentRoot[:])
		if err != nil {
			return nil, errors.Wrap(err, "could not get parent block")
		}
		if block == nil {
			return nil, errors.New("parent block is nil")
		}

		// Get the number of sync committee signatures
		numOfSyncCommitteeSignatures = 0
		if syncAggregate, err := block.Block().Body().SyncAggregate(); err == nil {
			numOfSyncCommitteeSignatures = syncAggregate.SyncCommitteeBits.Count()
		}
	}

	return block, nil
}
