package validator

import (
	"testing"

	"github.com/prysmaticlabs/prysm/v5/config/params"
	"github.com/prysmaticlabs/prysm/v5/consensus-types/blocks"
	"github.com/prysmaticlabs/prysm/v5/consensus-types/interfaces"
	"github.com/prysmaticlabs/prysm/v5/consensus-types/primitives"
	ethpb "github.com/prysmaticlabs/prysm/v5/proto/prysm/v1alpha1"
	"github.com/prysmaticlabs/prysm/v5/testing/require"
)

func Test_getEmptyBlock(t *testing.T) {
	params.SetupTestConfigCleanup(t)
	config := params.BeaconConfig()
	config.AltairForkEpoch = 1
	config.BellatrixForkEpoch = 2
	config.CapellaForkEpoch = 3
	config.DenebForkEpoch = 4
	config.ElectraForkEpoch = 5
	config.FuluForkEpoch = 6
	params.OverrideBeaconConfig(config)

	tests := []struct {
		name string
		slot primitives.Slot
		want func() interfaces.ReadOnlySignedBeaconBlock
	}{
		{
			name: "altair",
			slot: primitives.Slot(params.BeaconConfig().AltairForkEpoch) * params.BeaconConfig().SlotsPerEpoch,
			want: func() interfaces.ReadOnlySignedBeaconBlock {
				b, err := blocks.NewSignedBeaconBlock(&ethpb.SignedBeaconBlockAltair{Block: &ethpb.BeaconBlockAltair{Body: &ethpb.BeaconBlockBodyAltair{}}})
				require.NoError(t, err)
				return b
			},
		},
		{
			name: "bellatrix",
			slot: primitives.Slot(params.BeaconConfig().BellatrixForkEpoch) * params.BeaconConfig().SlotsPerEpoch,
			want: func() interfaces.ReadOnlySignedBeaconBlock {
				b, err := blocks.NewSignedBeaconBlock(&ethpb.SignedBeaconBlockBellatrix{Block: &ethpb.BeaconBlockBellatrix{Body: &ethpb.BeaconBlockBodyBellatrix{}}})
				require.NoError(t, err)
				return b
			},
		},
		{
			name: "capella",
			slot: primitives.Slot(params.BeaconConfig().CapellaForkEpoch) * params.BeaconConfig().SlotsPerEpoch,
			want: func() interfaces.ReadOnlySignedBeaconBlock {
				b, err := blocks.NewSignedBeaconBlock(&ethpb.SignedBeaconBlockCapella{Block: &ethpb.BeaconBlockCapella{Body: &ethpb.BeaconBlockBodyCapella{}}})
				require.NoError(t, err)
				return b
			},
		},
		{
			name: "deneb",
			slot: primitives.Slot(params.BeaconConfig().DenebForkEpoch) * params.BeaconConfig().SlotsPerEpoch,
			want: func() interfaces.ReadOnlySignedBeaconBlock {
				b, err := blocks.NewSignedBeaconBlock(&ethpb.SignedBeaconBlockDeneb{Block: &ethpb.BeaconBlockDeneb{Body: &ethpb.BeaconBlockBodyDeneb{}}})
				require.NoError(t, err)
				return b
			},
		},
		{
			name: "electra",
			slot: primitives.Slot(params.BeaconConfig().ElectraForkEpoch) * params.BeaconConfig().SlotsPerEpoch,
			want: func() interfaces.ReadOnlySignedBeaconBlock {
				b, err := blocks.NewSignedBeaconBlock(&ethpb.SignedBeaconBlockElectra{Block: &ethpb.BeaconBlockElectra{Body: &ethpb.BeaconBlockBodyElectra{}}})
				require.NoError(t, err)
				return b
			},
		},
		{
			name: "fulu",
			slot: primitives.Slot(params.BeaconConfig().FuluForkEpoch) * params.BeaconConfig().SlotsPerEpoch,
			want: func() interfaces.ReadOnlySignedBeaconBlock {
				b, err := blocks.NewSignedBeaconBlock(&ethpb.SignedBeaconBlockFulu{Block: &ethpb.BeaconBlockFulu{Body: &ethpb.BeaconBlockBodyFulu{}}})
				require.NoError(t, err)
				return b
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := getEmptyBlock(tt.slot)
			require.NoError(t, err)
			require.DeepEqual(t, tt.want(), got, "getEmptyBlock() = %v, want %v", got, tt.want())
		})
	}
}
