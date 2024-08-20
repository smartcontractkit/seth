package seth_test

import (
	"github.com/stretchr/testify/require"
	"testing"

	"github.com/smartcontractkit/seth"
)

func TestConfigAppendPkToEmptyNetwork(t *testing.T) {
	networkName := "network"
	cfg := &seth.Config{
		Network: &seth.Network{
			Name: networkName,
		},
	}

	added := cfg.AppendPksToNetwork([]string{"pk"}, networkName)
	require.True(t, added, "should have added pk to network")
	require.Equal(t, []string{"pk"}, cfg.Network.PrivateKeys, "network should have 1 pk")
}

func TestConfigAppendPkToEmptySharedNetwork(t *testing.T) {
	networkName := "network"
	network := &seth.Network{
		Name: networkName,
	}
	cfg := &seth.Config{
		Network:  network,
		Networks: []*seth.Network{network},
	}

	added := cfg.AppendPksToNetwork([]string{"pk"}, networkName)
	require.True(t, added, "should have added pk to network")
	require.Equal(t, []string{"pk"}, cfg.Network.PrivateKeys, "network should have 1 pk")
	require.Equal(t, []string{"pk"}, cfg.Networks[0].PrivateKeys, "network should have 1 pk")
}

func TestConfigAppendPkToNetworkWithPk(t *testing.T) {
	networkName := "network"
	cfg := &seth.Config{
		Network: &seth.Network{
			Name:        networkName,
			PrivateKeys: []string{"pk1"},
		},
	}

	added := cfg.AppendPksToNetwork([]string{"pk2"}, networkName)
	require.True(t, added, "should have added pk to network")
	require.Equal(t, []string{"pk1", "pk2"}, cfg.Network.PrivateKeys, "network should have 2 pks")
}

func TestConfigAppendPkToMissingNetwork(t *testing.T) {
	networkName := "network"
	cfg := &seth.Config{
		Network: &seth.Network{
			Name: "some_other",
		},
	}

	added := cfg.AppendPksToNetwork([]string{"pk"}, networkName)
	require.False(t, added, "should have not added pk to network")
	require.Equal(t, 0, len(cfg.Network.PrivateKeys), "network should have 0 pks")
}

func TestConfigAppendPkToInactiveNetwork(t *testing.T) {
	networkName := "network"
	cfg := &seth.Config{
		Network: &seth.Network{
			Name: "some_other",
		},
		Networks: []*seth.Network{
			{
				Name: "some_other",
			},
			{
				Name: networkName,
			},
		},
	}

	added := cfg.AppendPksToNetwork([]string{"pk"}, networkName)
	require.True(t, added, "should have added pk to network")
	require.Equal(t, 0, len(cfg.Network.PrivateKeys), "network should have 0 pks")
	require.Equal(t, 0, len(cfg.Networks[0].PrivateKeys), "network should have 0 pks")
	require.Equal(t, []string{"pk"}, cfg.Networks[1].PrivateKeys, "network should have 1 pk")
}
