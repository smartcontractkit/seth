package seth_test

import (
	"math/big"
	"os"
	"testing"

	"github.com/smartcontractkit/seth"
	"github.com/stretchr/testify/require"
)

// All you need to do is enable automated tracing and then wrap you contract call with `c.Decode`.
// This will automatically trace the transaction and decode it for you
func TestDecodeExample(t *testing.T) {
	_ = os.Setenv(seth.KEYFILE_PATH_ENV_VAR, "keyfile_test.toml")
	t.Cleanup(func() {
		os.Unsetenv(seth.KEYFILE_PATH_ENV_VAR)
	})
	contract := setup(t)
	c, err := seth.NewClient()
	require.NoError(t, err, "failed to initalise seth")

	// when this level is set we don't need to call TraceGethTX, because it's called automatically
	c.Cfg.TracingLevel = seth.TracingLevel_All

	_, err = c.Decode(contract.TraceDifferent(c.NewTXOpts(), big.NewInt(1), big.NewInt(2)))
	require.NoError(t, err, "failed to decode transaction")
}
