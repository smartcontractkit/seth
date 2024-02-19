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
	_ = os.Setenv("SETH_KEYFILE_PATH", "keyfile_test.toml")
	t.Cleanup(func() {
		os.Unsetenv("SETH_KEYFILE_PATH")
	})
	contract := setup(t)
	c, err := seth.NewClient()
	require.NoError(t, err, "failed to initalise seth")

	// when this flag is enabled we don't need to call TraceGethTX, because it's called automatically
	c.Cfg.TracingEnabled = true

	_, err = c.Decode(contract.TraceDifferent(c.NewTXOpts(), big.NewInt(1), big.NewInt(2)))
	require.NoError(t, err, "failed to decode transaction")
}
