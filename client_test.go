package seth_test

import (
	"github.com/ethereum/go-ethereum/common"
	link_token "github.com/smartcontractkit/seth/contracts/bind/link"
	"net/http"
	"testing"

	"github.com/smartcontractkit/seth"
	"github.com/stretchr/testify/require"
)

func TestRPCHealtCheckEnabled_Node_OK(t *testing.T) {
	cfg, err := seth.ReadConfig()
	require.NoError(t, err, "failed to read config")
	cfg.CheckRpcHealthOnStart = true

	_, err = seth.NewClientWithConfig(cfg)
	require.NoError(t, err, "failed to initalise seth")
}

func TestRPCHealtCheckDisabled_Node_OK(t *testing.T) {
	cfg, err := seth.ReadConfig()
	require.NoError(t, err, "failed to read config")
	cfg.CheckRpcHealthOnStart = false

	_, err = seth.NewClientWithConfig(cfg)
	require.NoError(t, err, "failed to initalise seth")
}

func TestRPCHealtCheckEnabled_Node_Unhealthy(t *testing.T) {
	cfg, err := seth.ReadConfig()
	require.NoError(t, err, "failed to read config")

	newPks, err := seth.NewEphemeralKeys(1)
	require.NoError(t, err, "failed to create ephemeral keys")

	cfg.CheckRpcHealthOnStart = true
	cfg.Network.PrivateKeys = []string{newPks[0]}

	_, err = seth.NewClientWithConfig(cfg)
	require.Error(t, err, "expected error when connecting to unhealthy node")
	require.Contains(t, err.Error(), seth.ErrRpcHealthCheckFailed, "expected error message when connecting to dead node")
}

func TestRPCHealtCheckDisabled_Node_Unhealthy(t *testing.T) {
	cfg, err := seth.ReadConfig()
	require.NoError(t, err, "failed to read config")

	newPks, err := seth.NewEphemeralKeys(1)
	require.NoError(t, err, "failed to create ephemeral keys")

	cfg.CheckRpcHealthOnStart = false
	cfg.Network.PrivateKeys = []string{newPks[0]}

	_, err = seth.NewClientWithConfig(cfg)
	require.NoError(t, err, "expected health check to be skipped")
}

func TestContractLoader(t *testing.T) {
	c, err := seth.NewClient()
	require.NoError(t, err, "failed to initalise seth")

	loader := seth.NewContractLoader[link_token.LinkToken](c)

	contract, err := loader.LoadContract("LinkToken", TestEnv.LinkTokenContract.Address(), link_token.LinkTokenMetaData.GetAbi, link_token.NewLinkToken)
	require.NoError(t, err, "failed to load contract")

	owner, err := contract.Owner(c.NewCallOpts())
	require.NoError(t, err, "failed to call loaded LINK contract")
	require.NotEqual(t, common.Address{}, owner, "expected owner to be set")
}

func TestRPCCustomHeadersFromEnv(t *testing.T) {
	tests := []struct {
		name            string
		headerEnvString string
		expected        http.Header
		expectedErr     error
	}{
		{
			name:            "single k-v",
			headerEnvString: "Host=http.com",
			expected:        http.Header{"Host": []string{"http.com"}},
		},
		{
			name:            "multiple k-v",
			headerEnvString: "Host=http.com,Accept=application/json",
			expected: http.Header{
				"Host":   []string{"http.com"},
				"Accept": []string{"application/json"},
			}},
		{
			name:            "empty value",
			headerEnvString: "Host=a,Accept",
			expectedErr:     seth.InvalidHeadersErr,
		},
		{
			name:            "invalid value",
			headerEnvString: "Host=a,Accept",
			expectedErr:     seth.InvalidHeadersErr,
		},
		{
			name:            "invalid k-v, multiple =",
			headerEnvString: "Host=a=b,Host=c",
			expectedErr:     seth.InvalidHeadersErr,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Setenv("SETH_RPC_HEADERS", tt.headerEnvString)
			result, err := seth.ReadEnvRPCHeaders()
			require.Equal(t, tt.expectedErr, err)
			require.Equal(t, tt.expected, result)
		})
	}
}
