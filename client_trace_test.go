package seth_test

import (
	"encoding/json"
	"fmt"
	"io"
	"math/big"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/barkimedes/go-deepcopy"
	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/crypto"
	"github.com/smartcontractkit/seth"
	network_debug_contract "github.com/smartcontractkit/seth/contracts/bind/debug"
	"github.com/stretchr/testify/require"
)

const (
	NoAnvilSupport = "Anvil doesn't support tracing"
	FailedToSend   = "failed to send transaction"
	FailedToMine   = "transaction mining failed"
	FailedToDecode = "failed to decode transaction"
	FailedToTrace  = "failed to trace transaction"
)

func SkipAnvil(t *testing.T, c *seth.Client) {
	if c.Cfg.Network.Name == "Anvil" {
		t.Skip(NoAnvilSupport)
	}
}

// since we uploaded the contracts via Seth, we have the contract address in the map
// and we can trace the calls correctly even though both calls have the same signature
func TestTraceTraceContractTracingSameMethodSignatures_UploadedViaSeth(t *testing.T) {
	c := newClientWithContractMapFromEnv(t)
	SkipAnvil(t, c)

	// when this level is set we don't need to call TraceGethTX, because it's called automatically
	c.Cfg.TracingLevel = seth.TracingLevel_All

	var x int64 = 2
	var y int64 = 4
	tx, err := c.Decode(TestEnv.DebugContract.Trace(c.NewTXOpts(), big.NewInt(x), big.NewInt(y)))
	require.NoError(t, err, FailedToDecode)

	require.Equal(t, 1, len(c.Tracer.DecodedCalls), "expected 1 decoded transacton")
	require.NotNil(t, c.Tracer.DecodedCalls[tx.Hash], "expected decoded calls to contain the transaction hash")
	require.Equal(t, 2, len(c.Tracer.DecodedCalls[tx.Hash]), "expected 2 decoded calls for this transaction")

	removeGasDataFromDecodedCalls(c.Tracer.DecodedCalls)

	firstExpectedCall := &seth.DecodedCall{
		FromAddress: strings.ToLower(c.Addresses[0].Hex()),
		ToAddress:   strings.ToLower(TestEnv.DebugContractAddress.Hex()),
		From:        "you",
		To:          "NetworkDebugContract",
		CommonData: seth.CommonData{
			Signature: "3e41f135",
			Method:    "trace(int256,int256)",
			Input:     map[string]interface{}{"x": big.NewInt(x), "y": big.NewInt(y)},
			Output:    map[string]interface{}{"0": big.NewInt(y + 2)},
		},
		Events: []seth.DecodedCommonLog{
			{
				Signature: "TwoIndexEvent(uint256,address)",
				EventData: map[string]interface{}{"roundId": big.NewInt(y), "startedBy": c.Addresses[0]},
				Address:   TestEnv.DebugContractAddress,
				Topics: []string{
					"0x33b47a1cd66813164ec00800d74296f57415217c22505ee380594a712936a0b5",
					"0x0000000000000000000000000000000000000000000000000000000000000004",
					"0x000000000000000000000000f39fd6e51aad88f6f4ce6ab8827279cfffb92266",
				},
			},
		},
		Comment: "",
	}

	require.EqualValues(t, firstExpectedCall, c.Tracer.DecodedCalls[tx.Hash][0], "first decoded call does not match")

	secondExpectedCall := &seth.DecodedCall{
		FromAddress: strings.ToLower(TestEnv.DebugContractAddress.Hex()),
		ToAddress:   strings.ToLower(TestEnv.DebugSubContractAddress.Hex()),
		From:        "NetworkDebugContract",
		To:          "NetworkDebugSubContract",
		CommonData: seth.CommonData{
			Signature: "3e41f135",
			Method:    "trace(int256,int256)",
			Input:     map[string]interface{}{"x": big.NewInt(x), "y": big.NewInt(y)},
			Output:    map[string]interface{}{"0": big.NewInt(y + 4)},
		},
		Comment: "",
	}

	actualSecondEvents := c.Tracer.DecodedCalls[tx.Hash][1].Events
	c.Tracer.DecodedCalls[tx.Hash][1].Events = nil

	require.EqualValues(t, secondExpectedCall, c.Tracer.DecodedCalls[tx.Hash][1], "second decoded call does not match")
	require.Equal(t, 1, len(actualSecondEvents), "second decoded call events count does not match")
	require.Equal(t, 3, len(actualSecondEvents[0].Topics), "second decoded event topics count does not match")

	expectedSecondEvents := []seth.DecodedCommonLog{
		{
			Signature: "TwoIndexEvent(uint256,address)",
			EventData: map[string]interface{}{"roundId": big.NewInt(6), "startedBy": TestEnv.DebugContractAddress},
			Address:   TestEnv.DebugSubContractAddress,
			Topics: []string{
				"0x33b47a1cd66813164ec00800d74296f57415217c22505ee380594a712936a0b5",
				"0x0000000000000000000000000000000000000000000000000000000000000006",
				// this one changes dynamically depending on sender address
				// "0x000000000000000000000000c351628eb244ec633d5f21fbd6621e1a683b1181",
			},
		},
	}
	actualSecondEvents[0].Topics = actualSecondEvents[0].Topics[0:2]
	require.EqualValues(t, expectedSecondEvents, actualSecondEvents, "second decoded call events do not match")
}

// we test a scenario, where because two contracts have the same method signature, both addresses
// were mapped to the same contract name (it doesn't happen always, it all depends on how data is ordered
// in the maps and that depends on addresses generated). We show that even if the initial mapping is incorrect,
// once we trace a transaction with different method signature, the mapping is corrected and the second transaction
// is traced correctly.
func TestTraceTraceContractTracingSameMethodSignatures_UploadedManually(t *testing.T) {
	c := newClient(t)
	SkipAnvil(t, c)

	for k := range c.ContractAddressToNameMap.GetContractMap() {
		delete(c.ContractAddressToNameMap.GetContractMap(), k)
	}

	// when this level is set we don't need to call TraceGethTX, because it's called automatically
	c.Cfg.TracingLevel = seth.TracingLevel_All

	// let's simulate this case, because it doesn't happen always, it all depends on the order of the
	// contract map, which is non-deterministic (hash map with keys being dynamically generated addresses)
	c.ContractAddressToNameMap.AddContract(TestEnv.DebugContractAddress.Hex(), "NetworkDebugContract")
	c.ContractAddressToNameMap.AddContract(TestEnv.DebugSubContractAddress.Hex(), "NetworkDebugContract")

	var x int64 = 2
	var y int64 = 4

	diffSigTx, txErr := c.Decode(TestEnv.DebugContract.TraceDifferent(c.NewTXOpts(), big.NewInt(x), big.NewInt(y)))
	require.NoError(t, txErr, FailedToDecode)
	sameSigTx, txErr := c.Decode(TestEnv.DebugContract.Trace(c.NewTXOpts(), big.NewInt(x), big.NewInt(y)))
	require.NoError(t, txErr, FailedToDecode)

	require.NotNil(t, c.Tracer.DecodedCalls[diffSigTx.Hash], "expected decoded calls to contain the diffSig transaction hash")
	require.Equal(t, 2, len(c.Tracer.DecodedCalls[diffSigTx.Hash]), "expected 2 decoded calls for diffSig transaction")

	removeGasDataFromDecodedCalls(c.Tracer.DecodedCalls)

	firstDiffSigCall := &seth.DecodedCall{
		FromAddress: strings.ToLower(c.Addresses[0].Hex()),
		ToAddress:   strings.ToLower(TestEnv.DebugContractAddress.Hex()),
		From:        "you",
		To:          "NetworkDebugContract",
		CommonData: seth.CommonData{
			Signature: "30985bcc",
			Method:    "traceDifferent(int256,int256)",
			Input:     map[string]interface{}{"x": big.NewInt(x), "y": big.NewInt(y)},
			Output:    map[string]interface{}{"0": big.NewInt(y + 2)},
		},
		Events: []seth.DecodedCommonLog{
			{
				Signature: "OneIndexEvent(uint256)",
				EventData: map[string]interface{}{"a": big.NewInt(x)},
				Address:   TestEnv.DebugContractAddress,
				Topics: []string{
					"0xeace1be0b97ec11f959499c07b9f60f0cc47bf610b28fda8fb0e970339cf3b35",
					"0x0000000000000000000000000000000000000000000000000000000000000002",
				},
			},
		},
		Comment: "",
	}

	require.EqualValues(t, firstDiffSigCall, c.Tracer.DecodedCalls[diffSigTx.Hash][0], "first diffSig decoded call does not match")

	secondDiffSigCall := &seth.DecodedCall{
		FromAddress: strings.ToLower(TestEnv.DebugContractAddress.Hex()),
		ToAddress:   strings.ToLower(TestEnv.DebugSubContractAddress.Hex()),
		From:        "NetworkDebugContract",
		To:          "NetworkDebugSubContract",
		CommonData: seth.CommonData{
			Signature: "047c4425",
			Method:    "traceOneInt(int256)",
			Input:     map[string]interface{}{"x": big.NewInt(x + 2)},
			Output:    map[string]interface{}{"r": big.NewInt(y + 3)},
		},
		Comment: "",
	}

	c.Tracer.DecodedCalls[diffSigTx.Hash][1].Events = nil
	require.EqualValues(t, secondDiffSigCall, c.Tracer.DecodedCalls[diffSigTx.Hash][1], "second diffSig decoded call does not match")

	require.Equal(t, 2, len(c.Tracer.DecodedCalls), "expected 2 decoded transactons")
	require.NotNil(t, c.Tracer.DecodedCalls[sameSigTx.Hash], "expected decoded calls to contain the sameSig transaction hash")
	require.Equal(t, 2, len(c.Tracer.DecodedCalls[sameSigTx.Hash]), "expected 2 decoded calls for sameSig transaction")

	firstSameSigCall := &seth.DecodedCall{
		FromAddress: strings.ToLower(c.Addresses[0].Hex()),
		ToAddress:   strings.ToLower(TestEnv.DebugContractAddress.Hex()),
		From:        "you",
		To:          "NetworkDebugContract",
		CommonData: seth.CommonData{
			Signature: "3e41f135",
			Method:    "trace(int256,int256)",
			Input:     map[string]interface{}{"x": big.NewInt(x), "y": big.NewInt(y)},
			Output:    map[string]interface{}{"0": big.NewInt(y + 2)},
		},
		Events: []seth.DecodedCommonLog{
			{
				Signature: "TwoIndexEvent(uint256,address)",
				EventData: map[string]interface{}{"roundId": big.NewInt(y), "startedBy": c.Addresses[0]},
				Address:   TestEnv.DebugContractAddress,
				Topics: []string{
					"0x33b47a1cd66813164ec00800d74296f57415217c22505ee380594a712936a0b5",
					"0x0000000000000000000000000000000000000000000000000000000000000004",
					"0x000000000000000000000000f39fd6e51aad88f6f4ce6ab8827279cfffb92266",
				},
			},
		},
		Comment: "",
	}

	require.EqualValues(t, firstSameSigCall, c.Tracer.DecodedCalls[sameSigTx.Hash][0], "first sameSig decoded call does not match")

	secondSameSigCall := &seth.DecodedCall{
		FromAddress: strings.ToLower(TestEnv.DebugContractAddress.Hex()),
		ToAddress:   strings.ToLower(TestEnv.DebugSubContractAddress.Hex()),
		From:        "NetworkDebugContract",
		To:          "NetworkDebugSubContract",
		CommonData: seth.CommonData{
			Signature: "3e41f135",
			Method:    "trace(int256,int256)",
			Input:     map[string]interface{}{"x": big.NewInt(x), "y": big.NewInt(y)},
			Output:    map[string]interface{}{"0": big.NewInt(y + 4)},
		},
	}

	actualSecondEvents := c.Tracer.DecodedCalls[sameSigTx.Hash][1].Events
	c.Tracer.DecodedCalls[sameSigTx.Hash][1].Events = nil

	require.EqualValues(t, secondSameSigCall, c.Tracer.DecodedCalls[sameSigTx.Hash][1], "second sameSig decoded call does not match")
	require.Equal(t, 1, len(actualSecondEvents), "second sameSig decoded call events count does not match")
	require.Equal(t, 3, len(actualSecondEvents[0].Topics), "second sameSig decoded event topics count does not match")

	expectedSecondEvents := []seth.DecodedCommonLog{
		{
			Signature: "TwoIndexEvent(uint256,address)",
			EventData: map[string]interface{}{"roundId": big.NewInt(6), "startedBy": TestEnv.DebugContractAddress},
			Address:   TestEnv.DebugSubContractAddress,
			Topics: []string{
				"0x33b47a1cd66813164ec00800d74296f57415217c22505ee380594a712936a0b5",
				"0x0000000000000000000000000000000000000000000000000000000000000006",
				// third topic changes dynamically depending on sender address
				// "0x000000000000000000000000c351628eb244ec633d5f21fbd6621e1a683b1181",
			},
		},
	}
	actualSecondEvents[0].Topics = actualSecondEvents[0].Topics[0:2]
	require.EqualValues(t, expectedSecondEvents, actualSecondEvents, "second sameSig decoded call events do not match")
}

func TestTraceTraceContractTracingSameMethodSignaturesWarningInComment_UploadedManually(t *testing.T) {
	c := newClient(t)
	SkipAnvil(t, c)

	c.ContractAddressToNameMap = seth.NewEmptyContractMap()

	// when this level is set we don't need to call TraceGethTX, because it's called automatically
	c.Cfg.TracingLevel = seth.TracingLevel_All

	sameSigTx, err := c.Decode(TestEnv.DebugContract.Trace(c.NewTXOpts(), big.NewInt(2), big.NewInt(2)))
	require.NoError(t, err, "failed to send transaction")

	require.NotNil(t, c.Tracer.DecodedCalls[sameSigTx.Hash], "expected decoded calls to contain the transaction hash")
	require.Equal(t, 2, len(c.Tracer.DecodedCalls[sameSigTx.Hash]), "expected 2 decoded calls for transaction")
	require.Equal(t, "potentially inaccurate - method present in 1 other contracts", c.Tracer.DecodedCalls[sameSigTx.Hash][1].Comment, "expected comment to be set")
}

// Here we show a certain tracing limitation, where contract A calls B, which calls A again.
// That call from B to A isn't present in call trace, but it's signature is present in 4bytes trace.
func TestTraceTraceContractTracingWithCallback_UploadedViaSeth(t *testing.T) {
	c := newClientWithContractMapFromEnv(t)
	SkipAnvil(t, c)

	// when this level is set we don't need to call TraceGethTX, because it's called automatically
	c.Cfg.TracingLevel = seth.TracingLevel_All

	// As this test might fail if run multiple times due to undterministic addressed in contract mapping
	// which sometime causes the call to be traced and sometimes not (it all depends on the order of
	// addresses in the map), I just remove potentially problematic ABI.
	delete((c.ContractStore.ABIs), "DebugContractCallback.abi")

	var x int64 = 2
	var y int64 = 4
	tx, txErr := c.Decode(TestEnv.DebugContract.TraceSubWithCallback(c.NewTXOpts(), big.NewInt(x), big.NewInt(y)))
	require.NoError(t, txErr, FailedToDecode)

	require.Equal(t, 1, len(c.Tracer.DecodedCalls), "expected 1 decoded transacton")
	require.NotNil(t, c.Tracer.DecodedCalls[tx.Hash], "expected decoded calls to contain the transaction hash")
	require.Equal(t, 3, len(c.Tracer.DecodedCalls[tx.Hash]), "expected 2 decoded calls for test transaction")

	removeGasDataFromDecodedCalls(c.Tracer.DecodedCalls)

	firstExpectedCall := &seth.DecodedCall{
		FromAddress: strings.ToLower(c.Addresses[0].Hex()),
		ToAddress:   strings.ToLower(TestEnv.DebugContractAddress.Hex()),
		From:        "you",
		To:          "NetworkDebugContract",
		CommonData: seth.CommonData{
			Signature: "3837a75e",
			Method:    "traceSubWithCallback(int256,int256)",
			Input:     map[string]interface{}{"x": big.NewInt(x), "y": big.NewInt(y)},
			Output:    map[string]interface{}{"0": big.NewInt(y + 4)},
		},
		Events: []seth.DecodedCommonLog{
			{
				Signature: "TwoIndexEvent(uint256,address)",
				EventData: map[string]interface{}{"roundId": big.NewInt(1), "startedBy": c.Addresses[0]},
				Address:   TestEnv.DebugContractAddress,
				Topics: []string{
					"0x33b47a1cd66813164ec00800d74296f57415217c22505ee380594a712936a0b5",
					"0x0000000000000000000000000000000000000000000000000000000000000001",
					"0x000000000000000000000000f39fd6e51aad88f6f4ce6ab8827279cfffb92266",
				},
			},
		},
		Comment: "",
	}

	require.EqualValues(t, firstExpectedCall, c.Tracer.DecodedCalls[tx.Hash][0], "first decoded call does not match")

	require.Equal(t, 2, len(c.Tracer.DecodedCalls[tx.Hash][1].Events), "second decoded call events count does not match")
	require.Equal(t, 3, len(c.Tracer.DecodedCalls[tx.Hash][1].Events[0].Topics), "second decoded first event topics count does not match")

	separatedTopcis := c.Tracer.DecodedCalls[tx.Hash][1].Events[0].Topics
	separatedTopcis = separatedTopcis[0:2]
	c.Tracer.DecodedCalls[tx.Hash][1].Events[0].Topics = nil

	secondExpectedCall := &seth.DecodedCall{
		FromAddress: strings.ToLower(TestEnv.DebugContractAddress.Hex()),
		ToAddress:   strings.ToLower(TestEnv.DebugSubContractAddress.Hex()),
		From:        "NetworkDebugContract",
		To:          "NetworkDebugSubContract",
		CommonData: seth.CommonData{
			Signature: "fa8fca7a",
			Method:    "traceWithCallback(int256,int256)",
			Input:     map[string]interface{}{"x": big.NewInt(x), "y": big.NewInt(y + 2)},
			Output:    map[string]interface{}{"0": big.NewInt(y + 2)},
		},
		Events: []seth.DecodedCommonLog{
			{
				Signature: "TwoIndexEvent(uint256,address)",
				EventData: map[string]interface{}{"roundId": big.NewInt(6), "startedBy": TestEnv.DebugContractAddress},
				Address:   TestEnv.DebugSubContractAddress,
			},
			{
				Signature: "OneIndexEvent(uint256)",
				EventData: map[string]interface{}{"a": big.NewInt(y + 2)},
				Address:   TestEnv.DebugSubContractAddress,
				Topics: []string{
					"0xeace1be0b97ec11f959499c07b9f60f0cc47bf610b28fda8fb0e970339cf3b35",
					"0x0000000000000000000000000000000000000000000000000000000000000006",
				},
			},
		},
		Comment: "",
	}

	require.EqualValues(t, secondExpectedCall, c.Tracer.DecodedCalls[tx.Hash][1], "second decoded call does not match")

	expectedTopics := []string{
		"0x33b47a1cd66813164ec00800d74296f57415217c22505ee380594a712936a0b5",
		"0x0000000000000000000000000000000000000000000000000000000000000006",
		// third topic is dynamic (sender address), skip it
		// "0x00000000000000000000000056fc17a65ccfec6b7ad0ade9bd9416cb365b9be8",
	}

	require.EqualValues(t, expectedTopics, separatedTopcis, "second decoded first event topics do not match")

	thirdExpectedCall := &seth.DecodedCall{
		FromAddress: seth.UNKNOWN,
		ToAddress:   strings.ToLower(TestEnv.DebugContractAddress.Hex()),
		From:        seth.UNKNOWN,
		To:          "NetworkDebugContract",
		CommonData: seth.CommonData{
			Signature: "0xfbcb8d07",
			Method:    "callbackMethod",
			Input:     map[string]interface{}{"warning": seth.NO_DATA},
			Output:    map[string]interface{}{"warning": seth.NO_DATA},
		},
		Events: []seth.DecodedCommonLog{
			{
				Signature: seth.NO_DATA,
				EventData: map[string]interface{}{"warning": seth.NO_DATA},
				Address:   common.Address{},
			},
		},
		Comment: seth.WrnMissingCallTrace,
	}
	require.EqualValues(t, thirdExpectedCall, c.Tracer.DecodedCalls[tx.Hash][2], "third decoded call does not match")
}

// Here we show that partial tracing works even if we don't have the ABI for the contract.
// We still try to decode what we can even without ABI and that we can decode the other call
// for which we do have ABI.
func TestTraceTraceContractTracingUnknownAbi(t *testing.T) {
	c := newClientWithContractMapFromEnv(t)
	SkipAnvil(t, c)

	// when this level is set we don't need to call TraceGethTX, because it's called automatically
	c.Cfg.TracingLevel = seth.TracingLevel_All

	// simulate missing ABI
	delete(c.ContractAddressToNameMap.GetContractMap(), strings.ToLower(TestEnv.DebugContractAddress.Hex()))
	delete(c.ContractStore.ABIs, "NetworkDebugContract.abi")

	var x int64 = 2
	var y int64 = 4
	tx, txErr := c.Decode(TestEnv.DebugContract.TraceDifferent(c.NewTXOpts(), big.NewInt(x), big.NewInt(y)))
	require.NoError(t, txErr, FailedToDecode)

	require.Equal(t, 1, len(c.Tracer.DecodedCalls), "expected 1 decoded transacton")
	require.NotNil(t, c.Tracer.DecodedCalls[tx.Hash], "expected decoded calls to contain the transaction hash")
	require.Equal(t, 2, len(c.Tracer.DecodedCalls[tx.Hash]), "expected 2 decoded calls for test transaction")

	removeGasDataFromDecodedCalls(c.Tracer.DecodedCalls)

	firstExpectedCall := &seth.DecodedCall{
		FromAddress: strings.ToLower(c.Addresses[0].Hex()),
		ToAddress:   strings.ToLower(TestEnv.DebugContractAddress.Hex()),
		From:        "you",
		To:          seth.UNKNOWN,
		CommonData: seth.CommonData{
			Signature: "30985bcc",
			Method:    seth.UNKNOWN,
			Input:     make(map[string]interface{}),
			Output:    make(map[string]interface{}),
		},
		Events:  []seth.DecodedCommonLog{},
		Comment: seth.CommentMissingABI,
	}

	require.EqualValues(t, firstExpectedCall, c.Tracer.DecodedCalls[tx.Hash][0], "first decoded call does not match")

	secondExpectedCall := &seth.DecodedCall{
		FromAddress: strings.ToLower(TestEnv.DebugContractAddress.Hex()),
		ToAddress:   strings.ToLower(TestEnv.DebugSubContractAddress.Hex()),
		From:        seth.UNKNOWN,
		To:          "NetworkDebugSubContract",
		CommonData: seth.CommonData{
			Signature: "047c4425",
			Method:    "traceOneInt(int256)",
			Input:     map[string]interface{}{"x": big.NewInt(x + 2)},
			Output:    map[string]interface{}{"r": big.NewInt(y + 3)},
		},
		Comment: "",
	}

	c.Tracer.DecodedCalls[tx.Hash][1].Events = nil
	require.EqualValues(t, secondExpectedCall, c.Tracer.DecodedCalls[tx.Hash][1], "second decoded call does not match")
}

func TestTraceTraceContractTracingNamedInputsAndOutputs(t *testing.T) {
	c := newClientWithContractMapFromEnv(t)
	SkipAnvil(t, c)

	// when this level is set we don't need to call TraceGethTX, because it's called automatically
	c.Cfg.TracingLevel = seth.TracingLevel_All

	x := big.NewInt(1000)
	var testString = "string"
	tx, txErr := c.Decode(TestEnv.DebugContract.EmitNamedInputsOutputs(c.NewTXOpts(), x, testString))
	require.NoError(t, txErr, FailedToDecode)

	require.Equal(t, 1, len(c.Tracer.DecodedCalls), "expected 1 decoded transacton")
	require.NotNil(t, c.Tracer.DecodedCalls[tx.Hash], "expected decoded calls to contain the transaction hash")

	expectedCall := &seth.DecodedCall{
		FromAddress: strings.ToLower(c.Addresses[0].Hex()),
		ToAddress:   strings.ToLower(TestEnv.DebugContractAddress.Hex()),
		From:        "you",
		To:          "NetworkDebugContract",
		CommonData: seth.CommonData{
			Signature: "45f0c9e6",
			Method:    "emitNamedInputsOutputs(uint256,string)",
			Input:     map[string]interface{}{"inputVal1": x, "inputVal2": testString},
			Output:    map[string]interface{}{"outputVal1": x, "outputVal2": testString},
		},
		Comment: "",
	}

	removeGasDataFromDecodedCalls(c.Tracer.DecodedCalls)
	require.EqualValues(t, expectedCall, c.Tracer.DecodedCalls[tx.Hash][0], "decoded call does not match")
}

func TestTraceTraceContractTracingNamedInputsAnonymousOutputs(t *testing.T) {
	c := newClientWithContractMapFromEnv(t)
	SkipAnvil(t, c)

	// when this level is set we don't need to call TraceGethTX, because it's called automatically
	c.Cfg.TracingLevel = seth.TracingLevel_All

	x := big.NewInt(1001)
	var testString = "string"
	tx, txErr := c.Decode(TestEnv.DebugContract.EmitInputsOutputs(c.NewTXOpts(), x, testString))
	require.NoError(t, txErr, "failed to send transaction")
	require.Equal(t, 1, len(c.Tracer.DecodedCalls), "expected 1 decoded transacton")
	require.NotNil(t, c.Tracer.DecodedCalls[tx.Hash], "expected decoded calls to contain the transaction hash")

	expectedCall := &seth.DecodedCall{
		FromAddress: strings.ToLower(c.Addresses[0].Hex()),
		ToAddress:   strings.ToLower(TestEnv.DebugContractAddress.Hex()),
		From:        "you",
		To:          "NetworkDebugContract",
		CommonData: seth.CommonData{
			Signature: "d7a80205",
			Method:    "emitInputsOutputs(uint256,string)",
			Input:     map[string]interface{}{"inputVal1": x, "inputVal2": testString},
			Output:    map[string]interface{}{"0": x, "1": testString},
		},
		Comment: "",
	}

	removeGasDataFromDecodedCalls(c.Tracer.DecodedCalls)
	require.EqualValues(t, expectedCall, c.Tracer.DecodedCalls[tx.Hash][0], "decoded call does not match")
}

// Shows that when output mixes named and unnamed paramaters, we can still decode the transaction,
// but that named outputs become unnamed and referenced by their index.
func TestTraceTraceContractTracingIntInputsWithoutLength(t *testing.T) {
	c := newClientWithContractMapFromEnv(t)
	SkipAnvil(t, c)

	// when this level is set we don't need to call TraceGethTX, because it's called automatically
	c.Cfg.TracingLevel = seth.TracingLevel_All

	x := big.NewInt(1001)
	y := big.NewInt(2)
	z := big.NewInt(26)
	tx, txErr := c.Decode(TestEnv.DebugContract.EmitInts(c.NewTXOpts(), x, y, z))
	require.NoError(t, txErr, "failed to send transaction")
	require.Equal(t, 1, len(c.Tracer.DecodedCalls), "expected 1 decoded transacton")
	require.NotNil(t, c.Tracer.DecodedCalls[tx.Hash], "expected decoded calls to contain the transaction hash")

	expectedCall := &seth.DecodedCall{
		FromAddress: strings.ToLower(c.Addresses[0].Hex()),
		ToAddress:   strings.ToLower(TestEnv.DebugContractAddress.Hex()),
		From:        "you",
		To:          "NetworkDebugContract",
		CommonData: seth.CommonData{
			Signature: "9e099652",
			Method:    "emitInts(int256,int128,uint256)",
			Input:     map[string]interface{}{"first": x, "second": y, "third": z},
			Output:    map[string]interface{}{"0": x, "1": y, "2": z},
		},
		Comment: "",
	}

	removeGasDataFromDecodedCalls(c.Tracer.DecodedCalls)
	require.EqualValues(t, expectedCall, c.Tracer.DecodedCalls[tx.Hash][0], "decoded call does not match")
}

func TestTraceTraceContractTracingAddressInputAndOutput(t *testing.T) {
	c := newClientWithContractMapFromEnv(t)
	SkipAnvil(t, c)

	// when this level is set we don't need to call TraceGethTX, because it's called automatically
	c.Cfg.TracingLevel = seth.TracingLevel_All

	address := c.Addresses[0]
	tx, txErr := c.Decode(TestEnv.DebugContract.EmitAddress(c.NewTXOpts(), address))
	require.NoError(t, txErr, "failed to send transaction")

	require.Equal(t, 1, len(c.Tracer.DecodedCalls), "expected 1 decoded transacton")
	require.NotNil(t, c.Tracer.DecodedCalls[tx.Hash], "expected decoded calls to contain the transaction hash")

	expectedCall := &seth.DecodedCall{
		FromAddress: strings.ToLower(c.Addresses[0].Hex()),
		ToAddress:   strings.ToLower(TestEnv.DebugContractAddress.Hex()),
		From:        "you",
		To:          "NetworkDebugContract",
		CommonData: seth.CommonData{
			Signature: "ec5c3ede",
			Method:    "emitAddress(address)",
			Input:     map[string]interface{}{"addr": address},
			Output:    map[string]interface{}{"0": address},
		},
		Comment: "",
	}

	removeGasDataFromDecodedCalls(c.Tracer.DecodedCalls)
	require.EqualValues(t, expectedCall, c.Tracer.DecodedCalls[tx.Hash][0], "decoded call does not match")
}

func TestTraceTraceContractTracingBytes32InputAndOutput(t *testing.T) {
	c := newClientWithContractMapFromEnv(t)
	SkipAnvil(t, c)

	// when this level is set we don't need to call TraceGethTX, because it's called automatically
	c.Cfg.TracingLevel = seth.TracingLevel_All

	addrAsBytes := c.Addresses[0].Bytes()
	addrAsBytes = append(addrAsBytes, c.Addresses[0].Bytes()...)
	var bytes32 [32]byte = [32]byte(addrAsBytes)
	tx, txErr := c.Decode(TestEnv.DebugContract.EmitBytes32(c.NewTXOpts(), bytes32))
	require.NoError(t, txErr, FailedToDecode)

	require.Equal(t, 1, len(c.Tracer.DecodedCalls), "expected 1 decoded transacton")
	require.NotNil(t, c.Tracer.DecodedCalls[tx.Hash], "expected decoded calls to contain the transaction hash")

	expectedCall := &seth.DecodedCall{
		FromAddress: strings.ToLower(c.Addresses[0].Hex()),
		ToAddress:   strings.ToLower(TestEnv.DebugContractAddress.Hex()),
		From:        "you",
		To:          "NetworkDebugContract",
		CommonData: seth.CommonData{
			Signature: "33311ef3",
			Method:    "emitBytes32(bytes32)",
			Input:     map[string]interface{}{"input": bytes32},
			Output:    map[string]interface{}{"output": bytes32},
		},
		Comment: "",
	}

	removeGasDataFromDecodedCalls(c.Tracer.DecodedCalls)
	require.EqualValues(t, expectedCall, c.Tracer.DecodedCalls[tx.Hash][0], "decoded call does not match")
}

func TestTraceTraceContractTracingUint256ArrayInputAndOutput(t *testing.T) {
	c := newClientWithContractMapFromEnv(t)
	SkipAnvil(t, c)

	// when this level is set we don't need to call TraceGethTX, because it's called automatically
	c.Cfg.TracingLevel = seth.TracingLevel_All

	uint256Array := []*big.Int{big.NewInt(1), big.NewInt(19271), big.NewInt(261), big.NewInt(271911), big.NewInt(821762721)}
	tx, txErr := c.Decode(TestEnv.DebugContract.ProcessUintArray(c.NewTXOpts(), uint256Array))
	require.NoError(t, txErr, FailedToDecode)
	require.Equal(t, 1, len(c.Tracer.DecodedCalls), "expected 1 decoded transacton")
	require.NotNil(t, c.Tracer.DecodedCalls[tx.Hash], "expected decoded calls to contain the transaction hash")

	output := []*big.Int{}
	for _, x := range uint256Array {
		output = append(output, big.NewInt(0).Add(x, big.NewInt(1)))
	}

	expectedCall := &seth.DecodedCall{
		FromAddress: strings.ToLower(c.Addresses[0].Hex()),
		ToAddress:   strings.ToLower(TestEnv.DebugContractAddress.Hex()),
		From:        "you",
		To:          "NetworkDebugContract",
		CommonData: seth.CommonData{
			Signature: "12d91233",
			Method:    "processUintArray(uint256[])",
			Input:     map[string]interface{}{"input": uint256Array},
			Output:    map[string]interface{}{"0": output},
		},
		Comment: "",
	}

	removeGasDataFromDecodedCalls(c.Tracer.DecodedCalls)
	require.EqualValues(t, expectedCall, c.Tracer.DecodedCalls[tx.Hash][0], "decoded call does not match")
}

func TestTraceTraceContractTracingAddressArrayInputAndOutput(t *testing.T) {
	c := newClientWithContractMapFromEnv(t)
	SkipAnvil(t, c)

	// when this level is set we don't need to call TraceGethTX, because it's called automatically
	c.Cfg.TracingLevel = seth.TracingLevel_All

	addressArray := []common.Address{c.Addresses[0], TestEnv.DebugSubContractAddress}
	tx, txErr := c.Decode(TestEnv.DebugContract.ProcessAddressArray(c.NewTXOpts(), addressArray))
	require.NoError(t, txErr, FailedToDecode)
	require.Equal(t, 1, len(c.Tracer.DecodedCalls), "expected 1 decoded transacton")
	require.NotNil(t, c.Tracer.DecodedCalls[tx.Hash], "expected decoded calls to contain the transaction hash")

	expectedCall := &seth.DecodedCall{
		FromAddress: strings.ToLower(c.Addresses[0].Hex()),
		ToAddress:   strings.ToLower(TestEnv.DebugContractAddress.Hex()),
		From:        "you",
		To:          "NetworkDebugContract",
		CommonData: seth.CommonData{
			Signature: "e1111f79",
			Method:    "processAddressArray(address[])",
			Input:     map[string]interface{}{"input": addressArray},
			Output:    map[string]interface{}{"0": addressArray},
		},
		Comment: "",
	}

	removeGasDataFromDecodedCalls(c.Tracer.DecodedCalls)
	require.EqualValues(t, expectedCall, c.Tracer.DecodedCalls[tx.Hash][0], "decoded call does not match")
}

func TestTraceTraceContractTracingStructWithDynamicFieldsInputAndOutput(t *testing.T) {
	c := newClientWithContractMapFromEnv(t)
	SkipAnvil(t, c)

	// when this level is set we don't need to call TraceGethTX, because it's called automatically
	c.Cfg.TracingLevel = seth.TracingLevel_All

	data := network_debug_contract.NetworkDebugContractData{
		Name:   "my awesome name",
		Values: []*big.Int{big.NewInt(2), big.NewInt(266810), big.NewInt(473878233)},
	}
	tx, txErr := c.Decode(TestEnv.DebugContract.ProcessDynamicData(c.NewTXOpts(), data))
	require.NoError(t, txErr, FailedToDecode)
	require.Equal(t, 1, len(c.Tracer.DecodedCalls), "expected 1 decoded transacton")
	require.NotNil(t, c.Tracer.DecodedCalls[tx.Hash], "expected decoded calls to contain the transaction hash")

	expected := struct {
		Name   string     `json:"name"`
		Values []*big.Int `json:"values"`
	}{
		Name:   data.Name,
		Values: data.Values,
	}

	expectedCall := &seth.DecodedCall{
		FromAddress: strings.ToLower(c.Addresses[0].Hex()),
		ToAddress:   strings.ToLower(TestEnv.DebugContractAddress.Hex()),
		From:        "you",
		To:          "NetworkDebugContract",
		CommonData: seth.CommonData{
			Signature: "7fdc8fe1",
			Method:    "processDynamicData((string,uint256[]))",
			Input:     map[string]interface{}{"data": expected},
			Output:    map[string]interface{}{"0": expected},
		},
		Comment: "",
	}

	removeGasDataFromDecodedCalls(c.Tracer.DecodedCalls)
	require.EqualValues(t, expectedCall, c.Tracer.DecodedCalls[tx.Hash][0], "decoded call does not match")
}

func TestTraceTraceContractTracingStructArrayWithDynamicFieldsInputAndOutput(t *testing.T) {
	c := newClientWithContractMapFromEnv(t)
	SkipAnvil(t, c)

	// when this level is set we don't need to call TraceGethTX, because it's called automatically
	c.Cfg.TracingLevel = seth.TracingLevel_All

	data := network_debug_contract.NetworkDebugContractData{
		Name:   "my awesome name",
		Values: []*big.Int{big.NewInt(2), big.NewInt(266810), big.NewInt(473878233)},
	}
	dataArray := [3]network_debug_contract.NetworkDebugContractData{data, data, data}
	tx, txErr := c.Decode(TestEnv.DebugContract.ProcessFixedDataArray(c.NewTXOpts(), dataArray))
	require.NoError(t, txErr, FailedToDecode)
	require.Equal(t, 1, len(c.Tracer.DecodedCalls), "expected 1 decoded transacton")
	require.NotNil(t, c.Tracer.DecodedCalls[tx.Hash], "expected decoded calls to contain the transaction hash")

	input := [3]struct {
		Name   string     `json:"name"`
		Values []*big.Int `json:"values"`
	}{
		{
			Name:   data.Name,
			Values: data.Values,
		},
		{
			Name:   data.Name,
			Values: data.Values,
		},
		{
			Name:   data.Name,
			Values: data.Values,
		},
	}

	output := [2]struct {
		Name   string     `json:"name"`
		Values []*big.Int `json:"values"`
	}{
		{
			Name:   data.Name,
			Values: data.Values,
		},
		{
			Name:   data.Name,
			Values: data.Values,
		},
	}

	expectedCall := &seth.DecodedCall{
		FromAddress: strings.ToLower(c.Addresses[0].Hex()),
		ToAddress:   strings.ToLower(TestEnv.DebugContractAddress.Hex()),
		From:        "you",
		To:          "NetworkDebugContract",
		CommonData: seth.CommonData{
			Signature: "99adad2e",
			Method:    "processFixedDataArray((string,uint256[])[3])",
			Input:     map[string]interface{}{"data": input},
			Output:    map[string]interface{}{"0": output},
		},
		Comment: "",
	}

	removeGasDataFromDecodedCalls(c.Tracer.DecodedCalls)
	require.EqualValues(t, expectedCall, c.Tracer.DecodedCalls[tx.Hash][0], "decoded call does not match")
}

func TestTraceTraceContractTracingNestedStructsWithDynamicFieldsInputAndOutput(t *testing.T) {
	c := newClientWithContractMapFromEnv(t)
	SkipAnvil(t, c)

	// when this level is set we don't need to call TraceGethTX, because it's called automatically
	c.Cfg.TracingLevel = seth.TracingLevel_All

	data := network_debug_contract.NetworkDebugContractNestedData{
		Data: network_debug_contract.NetworkDebugContractData{
			Name:   "my awesome name",
			Values: []*big.Int{big.NewInt(2), big.NewInt(266810), big.NewInt(473878233)},
		},
		DynamicBytes: []byte("dynamic bytes"),
	}
	tx, txErr := c.Decode(TestEnv.DebugContract.ProcessNestedData(c.NewTXOpts(), data))
	require.NoError(t, txErr, "failed to send transaction")
	require.Equal(t, 1, len(c.Tracer.DecodedCalls), "expected 1 decoded transacton")
	require.NotNil(t, c.Tracer.DecodedCalls[tx.Hash], "expected decoded calls to contain the transaction hash")

	input := struct {
		Data struct {
			Name   string     `json:"name"`
			Values []*big.Int `json:"values"`
		} `json:"data"`
		DynamicBytes []byte `json:"dynamicBytes"`
	}{
		struct {
			Name   string     `json:"name"`
			Values []*big.Int `json:"values"`
		}{
			Name:   data.Data.Name,
			Values: data.Data.Values,
		},
		data.DynamicBytes,
	}

	expectedCall := &seth.DecodedCall{
		FromAddress: strings.ToLower(c.Addresses[0].Hex()),
		ToAddress:   strings.ToLower(TestEnv.DebugContractAddress.Hex()),
		From:        "you",
		To:          "NetworkDebugContract",
		CommonData: seth.CommonData{
			Signature: "7f12881c",
			Method:    "processNestedData(((string,uint256[]),bytes))",
			Input:     map[string]interface{}{"data": input},
			Output:    map[string]interface{}{"0": input},
		},
		Comment: "",
	}

	removeGasDataFromDecodedCalls(c.Tracer.DecodedCalls)
	require.EqualValues(t, expectedCall, c.Tracer.DecodedCalls[tx.Hash][0], "decoded call does not match")
}

func TestTraceTraceContractTracingNestedStructsWithDynamicFieldsInputAndStructOutput(t *testing.T) {
	c := newClientWithContractMapFromEnv(t)
	SkipAnvil(t, c)

	// when this level is set we don't need to call TraceGethTX, because it's called automatically
	c.Cfg.TracingLevel = seth.TracingLevel_All

	data := network_debug_contract.NetworkDebugContractData{
		Name:   "my awesome name",
		Values: []*big.Int{big.NewInt(2), big.NewInt(266810), big.NewInt(473878233)},
	}
	tx, txErr := c.Decode(TestEnv.DebugContract.ProcessNestedData0(c.NewTXOpts(), data))
	require.NoError(t, txErr, "failed to send transaction")
	require.Equal(t, 1, len(c.Tracer.DecodedCalls), "expected 1 decoded transacton")
	require.NotNil(t, c.Tracer.DecodedCalls[tx.Hash], "expected decoded calls to contain the transaction hash")

	input := struct {
		Name   string     `json:"name"`
		Values []*big.Int `json:"values"`
	}{
		Name:   data.Name,
		Values: data.Values,
	}

	hash := crypto.Keccak256Hash([]byte(input.Name))

	output := struct {
		Data struct {
			Name   string     `json:"name"`
			Values []*big.Int `json:"values"`
		} `json:"data"`
		DynamicBytes []byte `json:"dynamicBytes"`
	}{
		struct {
			Name   string     `json:"name"`
			Values []*big.Int `json:"values"`
		}{
			Name:   data.Name,
			Values: data.Values,
		},
		hash.Bytes(),
	}

	expectedCall := &seth.DecodedCall{
		FromAddress: strings.ToLower(c.Addresses[0].Hex()),
		ToAddress:   strings.ToLower(TestEnv.DebugContractAddress.Hex()),
		From:        "you",
		To:          "NetworkDebugContract",
		CommonData: seth.CommonData{
			Signature: "f499af2a",
			Method:    "processNestedData((string,uint256[]))",
			Input:     map[string]interface{}{"data": input},
			Output:    map[string]interface{}{"0": output},
		},
		Comment: "",
	}

	removeGasDataFromDecodedCalls(c.Tracer.DecodedCalls)
	require.EqualValues(t, expectedCall, c.Tracer.DecodedCalls[tx.Hash][0], "decoded call does not match")
}

func TestTraceTraceContractTracingPayable(t *testing.T) {
	c := newClientWithContractMapFromEnv(t)
	SkipAnvil(t, c)

	// when this level is set we don't need to call TraceGethTX, because it's called automatically
	c.Cfg.TracingLevel = seth.TracingLevel_All

	var value int64 = 1000
	tx, txErr := c.Decode(TestEnv.DebugContract.Pay(c.NewTXOpts(seth.WithValue(big.NewInt(value)))))
	require.NoError(t, txErr, FailedToDecode)
	require.Equal(t, 1, len(c.Tracer.DecodedCalls), "expected 1 decoded transacton")
	require.NotNil(t, c.Tracer.DecodedCalls[tx.Hash], "expected decoded calls to contain the transaction hash")

	expectedCall := &seth.DecodedCall{
		FromAddress: strings.ToLower(c.Addresses[0].Hex()),
		ToAddress:   strings.ToLower(TestEnv.DebugContractAddress.Hex()),
		From:        "you",
		To:          "NetworkDebugContract",
		CommonData: seth.CommonData{
			Signature: "1b9265b8",
			Method:    "pay()",
			Output:    map[string]interface{}{},
		},
		Comment: "",
		Value:   value,
	}

	removeGasDataFromDecodedCalls(c.Tracer.DecodedCalls)
	require.EqualValues(t, expectedCall, c.Tracer.DecodedCalls[tx.Hash][0], "decoded call does not match")
}

func TestTraceTraceContractTracingFallback(t *testing.T) {
	t.Skip("Need to investigate further how to support it, the call succeds, but we fail to decode it")
	// our ABIFinder doesn't know anything about fallback, but maybe we should use it, when everything else fails?
	c := newClientWithContractMapFromEnv(t)
	SkipAnvil(t, c)

	// when this level is set we don't need to call TraceGethTX, because it's called automatically
	c.Cfg.TracingLevel = seth.TracingLevel_All

	tx, txErr := c.Decode(TestEnv.DebugContractRaw.RawTransact(c.NewTXOpts(), []byte("iDontExist")))
	require.NoError(t, txErr, FailedToDecode)
	require.Equal(t, 1, len(c.Tracer.DecodedCalls), "expected 1 decoded transacton")
	require.NotNil(t, c.Tracer.DecodedCalls[tx.Hash], "expected decoded calls to contain the transaction hash")

	expectedCall := &seth.DecodedCall{
		FromAddress: strings.ToLower(c.Addresses[0].Hex()),
		ToAddress:   strings.ToLower(TestEnv.DebugContractAddress.Hex()),
		From:        "you",
		To:          "NetworkDebugContract",
		CommonData: seth.CommonData{
			Signature: "1b9265b8",
			Method:    "pay()",
			Output:    map[string]interface{}{},
		},
		Comment: "",
	}

	removeGasDataFromDecodedCalls(c.Tracer.DecodedCalls)
	require.EqualValues(t, expectedCall, c.Tracer.DecodedCalls[tx.Hash][0], "decoded call does not match")
}

func TestTraceTraceContractTracingReceive(t *testing.T) {
	t.Skip("Need to investigate further how to support it, the call succeds, but we fail to match the signature as input is 0x")
	c := newClientWithContractMapFromEnv(t)
	SkipAnvil(t, c)

	// when this level is set we don't need to call TraceGethTX, because it's called automatically
	c.Cfg.TracingLevel = seth.TracingLevel_All

	value := big.NewInt(29121)
	tx, txErr := c.Decode(TestEnv.DebugContract.Receive(c.NewTXOpts(seth.WithValue(value))))
	require.NoError(t, txErr, FailedToDecode)
	require.Equal(t, 1, len(c.Tracer.DecodedCalls), "expected 1 decoded transacton")
	require.NotNil(t, c.Tracer.DecodedCalls[tx.Hash], "expected decoded calls to contain the transaction hash")

	expectedCall := &seth.DecodedCall{
		FromAddress: strings.ToLower(c.Addresses[0].Hex()),
		ToAddress:   strings.ToLower(TestEnv.DebugContractAddress.Hex()),
		From:        "you",
		To:          "NetworkDebugContract",
		CommonData: seth.CommonData{
			Signature: "1b9265b8",
			Method:    "pay()",
			Output:    map[string]interface{}{},
		},
		Comment: "",
	}

	removeGasDataFromDecodedCalls(c.Tracer.DecodedCalls)
	require.EqualValues(t, expectedCall, c.Tracer.DecodedCalls[tx.Hash][0], "decoded call does not match")
}

func TestTraceTraceContractTracingEnumInputAndOutput(t *testing.T) {
	c := newClientWithContractMapFromEnv(t)
	SkipAnvil(t, c)

	// when this level is set we don't need to call TraceGethTX, because it's called automatically
	c.Cfg.TracingLevel = seth.TracingLevel_All

	var status uint8 = 1 // Active
	tx, txErr := c.Decode(TestEnv.DebugContract.SetStatus(c.NewTXOpts(), status))
	require.NoError(t, txErr, FailedToDecode)
	require.Equal(t, 1, len(c.Tracer.DecodedCalls), "expected 1 decoded transacton")
	require.NotNil(t, c.Tracer.DecodedCalls[tx.Hash], "expected decoded calls to contain the transaction hash")

	expectedCall := &seth.DecodedCall{
		FromAddress: strings.ToLower(c.Addresses[0].Hex()),
		ToAddress:   strings.ToLower(TestEnv.DebugContractAddress.Hex()),
		From:        "you",
		To:          "NetworkDebugContract",
		CommonData: seth.CommonData{
			Signature: "2e49d78b",
			Method:    "setStatus(uint8)",
			Input:     map[string]interface{}{"status": status},
			Output:    map[string]interface{}{"0": status},
		},
		Comment: "",
		Events: []seth.DecodedCommonLog{
			{
				Signature: "CurrentStatus(uint8)",
				EventData: map[string]interface{}{"status": status},
				Address:   TestEnv.DebugContractAddress,
				Topics: []string{
					"0xbea054406fdf249b05d1aef1b5f848d62d902d94389fca702b2d8337677c359a",
					"0x0000000000000000000000000000000000000000000000000000000000000001",
				},
			},
		},
	}

	removeGasDataFromDecodedCalls(c.Tracer.DecodedCalls)
	require.EqualValues(t, expectedCall, c.Tracer.DecodedCalls[tx.Hash][0], "decoded call does not match")
}

func TestTraceTraceContractTracingNonIndexedEventParameter(t *testing.T) {
	c := newClientWithContractMapFromEnv(t)
	SkipAnvil(t, c)

	// when this level is set we don't need to call TraceGethTX, because it's called automatically
	c.Cfg.TracingLevel = seth.TracingLevel_All

	tx, txErr := c.Decode(TestEnv.DebugContract.EmitNoIndexEventString(c.NewTXOpts()))
	require.NoError(t, txErr, "failed to send transaction")
	require.Equal(t, 1, len(c.Tracer.DecodedCalls), "expected 1 decoded transacton")
	require.NotNil(t, c.Tracer.DecodedCalls[tx.Hash], "expected decoded calls to contain the transaction hash")

	expectedCall := &seth.DecodedCall{
		FromAddress: strings.ToLower(c.Addresses[0].Hex()),
		ToAddress:   strings.ToLower(TestEnv.DebugContractAddress.Hex()),
		From:        "you",
		To:          "NetworkDebugContract",
		CommonData: seth.CommonData{
			Signature: "788c4772",
			Method:    "emitNoIndexEventString()",
			Input:     nil,
			Output:    map[string]interface{}{},
		},
		Comment: "",
		Events: []seth.DecodedCommonLog{
			{
				Signature: "NoIndexEventString(string)",
				EventData: map[string]interface{}{"str": "myString"},
				Address:   TestEnv.DebugContractAddress,
				Topics: []string{
					"0x25b7adba1b046a19379db4bc06aa1f2e71604d7b599a0ee8783d58110f00e16a",
				},
			},
		},
	}

	removeGasDataFromDecodedCalls(c.Tracer.DecodedCalls)
	require.EqualValues(t, expectedCall, c.Tracer.DecodedCalls[tx.Hash][0], "decoded call does not match")
}

func TestTraceTraceContractTracingEventThreeIndexedParameters(t *testing.T) {
	c := newClientWithContractMapFromEnv(t)
	SkipAnvil(t, c)

	// when this level is set we don't need to call TraceGethTX, because it's called automatically
	c.Cfg.TracingLevel = seth.TracingLevel_All

	tx, txErr := c.Decode(TestEnv.DebugContract.EmitThreeIndexEvent(c.NewTXOpts()))
	require.NoError(t, txErr, FailedToDecode)
	require.Equal(t, 1, len(c.Tracer.DecodedCalls), "expected 1 decoded transacton")
	require.NotNil(t, c.Tracer.DecodedCalls[tx.Hash], "expected decoded calls to contain the transaction hash")

	expectedCall := &seth.DecodedCall{
		FromAddress: strings.ToLower(c.Addresses[0].Hex()),
		ToAddress:   strings.ToLower(TestEnv.DebugContractAddress.Hex()),
		From:        "you",
		To:          "NetworkDebugContract",
		CommonData: seth.CommonData{
			Signature: "aa3fdcf4",
			Method:    "emitThreeIndexEvent()",
			Input:     nil,
			Output:    map[string]interface{}{},
		},
		Comment: "",
		Events: []seth.DecodedCommonLog{
			{
				Signature: "ThreeIndexEvent(uint256,address,uint256)",
				EventData: map[string]interface{}{"roundId": big.NewInt(1), "startedAt": big.NewInt(3), "startedBy": c.Addresses[0]},
				Address:   TestEnv.DebugContractAddress,
				Topics: []string{
					"0x5660e8f93f0146f45abcd659e026b75995db50053cbbca4d7f365934ade68bf3",
					"0x0000000000000000000000000000000000000000000000000000000000000001",
					"0x000000000000000000000000f39fd6e51aad88f6f4ce6ab8827279cfffb92266",
					"0x0000000000000000000000000000000000000000000000000000000000000003",
				},
			},
		},
	}

	removeGasDataFromDecodedCalls(c.Tracer.DecodedCalls)
	require.EqualValues(t, expectedCall, c.Tracer.DecodedCalls[tx.Hash][0], "decoded call does not match")
}

func TestTraceTraceContractTracingEventFourMixedParameters(t *testing.T) {
	c := newClientWithContractMapFromEnv(t)
	SkipAnvil(t, c)

	// when this level is set we don't need to call TraceGethTX, because it's called automatically
	c.Cfg.TracingLevel = seth.TracingLevel_All

	tx, txErr := c.Decode(TestEnv.DebugContract.EmitFourParamMixedEvent(c.NewTXOpts()))
	require.NoError(t, txErr, FailedToDecode)
	require.Equal(t, 1, len(c.Tracer.DecodedCalls), "expected 1 decoded transacton")
	require.NotNil(t, c.Tracer.DecodedCalls[tx.Hash], "expected decoded calls to contain the transaction hash")

	expectedCall := &seth.DecodedCall{
		FromAddress: strings.ToLower(c.Addresses[0].Hex()),
		ToAddress:   strings.ToLower(TestEnv.DebugContractAddress.Hex()),
		From:        "you",
		To:          "NetworkDebugContract",
		CommonData: seth.CommonData{
			Signature: "c2124b22",
			Method:    "emitFourParamMixedEvent()",
			Input:     nil,
			Output:    map[string]interface{}{},
		},
		Comment: "",
		Events: []seth.DecodedCommonLog{
			{
				Signature: "ThreeIndexAndOneNonIndexedEvent(uint256,address,uint256,string)",
				EventData: map[string]interface{}{"roundId": big.NewInt(2), "startedAt": big.NewInt(3), "startedBy": c.Addresses[0], "dataId": "some id"},
				Address:   TestEnv.DebugContractAddress,
				Topics: []string{
					"0x56c2ea44ba516098cee0c181dd9d8db262657368b6e911e83ae0ccfae806c73d",
					"0x0000000000000000000000000000000000000000000000000000000000000002",
					"0x000000000000000000000000f39fd6e51aad88f6f4ce6ab8827279cfffb92266",
					"0x0000000000000000000000000000000000000000000000000000000000000003",
				},
			},
		},
	}

	removeGasDataFromDecodedCalls(c.Tracer.DecodedCalls)
	require.EqualValues(t, expectedCall, c.Tracer.DecodedCalls[tx.Hash][0], "decoded call does not match")
}

func TestTraceTraceContractTraceAll(t *testing.T) {
	c := newClientWithContractMapFromEnv(t)
	SkipAnvil(t, c)

	// when this level is set we don't need to call TraceGethTX, because it's automatically executed for all transactions
	c.Cfg.TracingLevel = seth.TracingLevel_All

	revertedTx, txErr := TestEnv.DebugContract.AlwaysRevertsCustomError(c.NewTXOpts())
	require.NoError(t, txErr, "transaction sending should not fail")
	_, decodeErr := c.Decode(revertedTx, txErr)
	require.Error(t, decodeErr, "transaction should have reverted")
	require.Equal(t, "error type: CustomErr, error values: [12 21]", decodeErr.Error(), "expected error message to contain the reverted error type and values")

	okTx, txErr := TestEnv.DebugContract.AddCounter(c.NewTXOpts(), big.NewInt(1), big.NewInt(2))
	require.NoError(t, txErr, "transaction should not have reverted")
	_, decodeErr = c.Decode(okTx, txErr)
	require.NoError(t, decodeErr, "transaction decoding should not err")
	require.Equal(t, 2, len(c.Tracer.DecodedCalls), "expected 1 decoded transacton")
	removeGasDataFromDecodedCalls(c.Tracer.DecodedCalls)

	expectedCall := &seth.DecodedCall{
		FromAddress: strings.ToLower(c.Addresses[0].Hex()),
		ToAddress:   strings.ToLower(TestEnv.DebugContractAddress.Hex()),
		From:        "you",
		To:          "NetworkDebugContract",
		CommonData: seth.CommonData{
			Signature: "5e9c80d6",
			Method:    "alwaysRevertsCustomError()",
			Output:    map[string]interface{}{},
		},
		Comment: "",
	}

	require.EqualValues(t, expectedCall, c.Tracer.DecodedCalls[revertedTx.Hash().Hex()][0], "reverted decoded call does not match")

	expectedCall = &seth.DecodedCall{
		FromAddress: strings.ToLower(c.Addresses[0].Hex()),
		ToAddress:   strings.ToLower(TestEnv.DebugContractAddress.Hex()),
		From:        "you",
		To:          "NetworkDebugContract",
		CommonData: seth.CommonData{
			Signature: "23515760",
			Method:    "addCounter(int256,int256)",
			Output:    map[string]interface{}{"value": big.NewInt(2)},
			Input:     map[string]interface{}{"idx": big.NewInt(1), "x": big.NewInt(2)},
		},
		Comment: "",
	}

	require.EqualValues(t, expectedCall, c.Tracer.DecodedCalls[okTx.Hash().Hex()][0], "successful decoded call does not match")
}

func TestTraceTraceContractTraceOnlyReverted(t *testing.T) {
	c := newClientWithContractMapFromEnv(t)
	SkipAnvil(t, c)

	// when this level is set we don't need to call TraceGethTX, because it's automatically executed for all reverted transactions
	c.Cfg.TracingLevel = seth.TracingLevel_Reverted

	revertedTx, txErr := TestEnv.DebugContract.AlwaysRevertsCustomError(c.NewTXOpts())
	require.NoError(t, txErr, "transaction sending should not fail")
	_, decodeErr := c.Decode(revertedTx, txErr)
	require.Error(t, decodeErr, "transaction should have reverted")
	require.Equal(t, "error type: CustomErr, error values: [12 21]", decodeErr.Error(), "expected error message to contain the reverted error type and values")

	okTx, txErr := TestEnv.DebugContract.AddCounter(c.NewTXOpts(), big.NewInt(1), big.NewInt(2))
	require.NoError(t, txErr, "transaction should not have reverted")
	_, decodeErr = c.Decode(okTx, txErr)
	require.NoError(t, decodeErr, "transaction decoding should not err")

	require.Equal(t, 1, len(c.Tracer.DecodedCalls), "expected 1 decoded transacton")

	expectedCall := &seth.DecodedCall{
		FromAddress: strings.ToLower(c.Addresses[0].Hex()),
		ToAddress:   strings.ToLower(TestEnv.DebugContractAddress.Hex()),
		From:        "you",
		To:          "NetworkDebugContract",
		CommonData: seth.CommonData{
			Signature: "5e9c80d6",
			Method:    "alwaysRevertsCustomError()",
			Output:    map[string]interface{}{},
		},
		Comment: "",
	}

	removeGasDataFromDecodedCalls(c.Tracer.DecodedCalls)
	require.EqualValues(t, expectedCall, c.Tracer.DecodedCalls[revertedTx.Hash().Hex()][0], "decoded call does not match")
}

func TestTraceTraceContractTraceNone(t *testing.T) {
	c := newClientWithContractMapFromEnv(t)
	SkipAnvil(t, c)

	// when this level nothing is ever traced or debugged
	c.Cfg.TracingLevel = seth.TracingLevel_None

	revertedTx, txErr := TestEnv.DebugContract.AlwaysRevertsCustomError(c.NewTXOpts())
	require.NoError(t, txErr, "transaction sending should not fail")
	_, decodeErr := c.Decode(revertedTx, txErr)
	require.Error(t, decodeErr, "transaction should have reverted")
	require.Equal(t, "error type: CustomErr, error values: [12 21]", decodeErr.Error(), "expected error message to contain the reverted error type and values")

	okTx, txErr := TestEnv.DebugContract.AddCounter(c.NewTXOpts(), big.NewInt(1), big.NewInt(2))
	require.NoError(t, txErr, "transaction should not have reverted")
	_, decodeErr = c.Decode(okTx, txErr)
	require.NoError(t, decodeErr, "transaction decoding should not err")

	require.Empty(t, c.Tracer.DecodedCalls, "expected 1 decoded transacton")
}

func TestCallRevertFunctionInTheContract(t *testing.T) {
	c := newClientWithContractMapFromEnv(t)
	SkipAnvil(t, c)

	// when this flag is enabled we don't need to call TraceGethTX, because it's called automatically
	c.Cfg.TracingEnabled = true
	c.TraceReverted = true

	tx, txErr := TestEnv.DebugContract.CallRevertFunctionInTheContract(c.NewTXOpts())
	require.NoError(t, txErr, "transaction should have reverted")
	_, decodeErr := c.Decode(tx, txErr)
	require.Error(t, decodeErr, "transaction should have reverted")
	require.Equal(t, "error type: CustomErr, error values: [12 21]", decodeErr.Error(), "expected error message to contain the reverted error type and values")
	require.Equal(t, 1, len(c.Tracer.DecodedCalls), "expected 1 decoded transacton")
}

func TestCallRevertFunctionInSubContract(t *testing.T) {
	c := newClientWithContractMapFromEnv(t)
	SkipAnvil(t, c)

	// when this flag is enabled we don't need to call TraceGethTX, because it's called automatically
	c.Cfg.TracingEnabled = true
	c.TraceReverted = true

	x := big.NewInt(1001)
	y := big.NewInt(2)
	tx, txErr := TestEnv.DebugContract.CallRevertFunctionInSubContract(c.NewTXOpts(), x, y)
	require.NoError(t, txErr, "transaction should have reverted")
	_, decodeErr := c.Decode(tx, txErr)
	require.Error(t, decodeErr, "transaction should have reverted")
	require.Equal(t, "error type: CustomErr, error values: [1001 2]", decodeErr.Error(), "expected error message to contain the reverted error type and values")
	require.Equal(t, 1, len(c.Tracer.DecodedCalls), "expected 1 decoded transacton")
}

func TestContractRevertedSubContract(t *testing.T) {
	c := newClientWithContractMapFromEnv(t)
	SkipAnvil(t, c)

	// when this flag is enabled we don't need to call TraceGethTX, because it's called automatically
	c.Cfg.TracingEnabled = true
	c.TraceReverted = true

	x := big.NewInt(1001)
	y := big.NewInt(2)
	tx, txErr := TestEnv.DebugContract.CallRevertFunctionInSubContract(c.NewTXOpts(), x, y)
	require.NoError(t, txErr, "transaction should have reverted")
	_, decodeErr := c.Decode(tx, txErr)
	require.Error(t, decodeErr, "transaction should have reverted")
	require.Equal(t, "error type: CustomErr, error values: [1001 2]", decodeErr.Error(), "expected error message to contain the reverted error type and values")
	require.Equal(t, 1, len(c.Tracer.DecodedCalls), "expected 1 decoded transacton")
}

func TestTraceTraceContractTracingClientIntialisesTracerIfTracingIsEnabled(t *testing.T) {
	cfg := deepcopy.MustAnything(TestEnv.Client.Cfg).(*seth.Config)

	as, err := seth.NewContractStore(filepath.Join(cfg.ConfigDir, cfg.ABIDir), filepath.Join(cfg.ConfigDir, cfg.BINDir))
	require.NoError(t, err, "failed to create contract store")

	nm, err := seth.NewNonceManager(cfg, TestEnv.Client.Addresses, TestEnv.Client.PrivateKeys)
	require.NoError(t, err, "failed to create nonce manager")

	// when this level is set we don't need to call TraceGethTX, because it's called automatically
	cfg.TracingLevel = seth.TracingLevel_All
	cfg.Network.TxnTimeout = seth.MustMakeDuration(time.Duration(5 * time.Second))

	c, err := seth.NewClientRaw(
		cfg,
		TestEnv.Client.Addresses,
		TestEnv.Client.PrivateKeys,
		seth.WithContractStore(as),
		seth.WithNonceManager(nm),
	)
	require.NoError(t, err, "failed to create client")
	SkipAnvil(t, c)

	x := big.NewInt(1001)
	y := big.NewInt(2)
	z := big.NewInt(26)
	tx, txErr := c.Decode(TestEnv.DebugContract.EmitInts(c.NewTXOpts(), x, y, z))
	require.NoError(t, txErr, "failed to send transaction")
	require.Equal(t, 1, len(c.Tracer.DecodedCalls), "expected 1 decoded transacton")
	require.NotNil(t, c.Tracer.DecodedCalls[tx.Hash], "expected decoded calls to contain the transaction hash")

	expectedCall := &seth.DecodedCall{
		FromAddress: strings.ToLower(c.Addresses[0].Hex()),
		ToAddress:   strings.ToLower(TestEnv.DebugContractAddress.Hex()),
		From:        "you",
		To:          "NetworkDebugContract",
		CommonData: seth.CommonData{
			Signature: "9e099652",
			Method:    "emitInts(int256,int128,uint256)",
			Input:     map[string]interface{}{"first": x, "second": y, "third": z},
			Output:    map[string]interface{}{"0": x, "1": y, "2": z},
		},
		Comment: "",
	}

	removeGasDataFromDecodedCalls(c.Tracer.DecodedCalls)
	require.EqualValues(t, expectedCall, c.Tracer.DecodedCalls[tx.Hash][0], "decoded call does not match")
}

func TestTraceTraceContractTracingSaveToJson(t *testing.T) {
	cfg := deepcopy.MustAnything(TestEnv.Client.Cfg).(*seth.Config)

	as, err := seth.NewContractStore(filepath.Join(cfg.ConfigDir, cfg.ABIDir), filepath.Join(cfg.ConfigDir, cfg.BINDir))
	require.NoError(t, err, "failed to create contract store")

	nm, err := seth.NewNonceManager(cfg, TestEnv.Client.Addresses, TestEnv.Client.PrivateKeys)
	require.NoError(t, err, "failed to create nonce manager")

	// when this level is set we don't need to call TraceGethTX, because it's called automatically
	cfg.TracingLevel = seth.TracingLevel_All
	cfg.TraceToJson = true
	cfg.Network.TxnTimeout = seth.MustMakeDuration(time.Duration(5 * time.Second))

	c, err := seth.NewClientRaw(
		cfg,
		TestEnv.Client.Addresses,
		TestEnv.Client.PrivateKeys,
		seth.WithContractStore(as),
		seth.WithNonceManager(nm),
	)
	require.NoError(t, err, "failed to create client")
	SkipAnvil(t, c)

	x := big.NewInt(1001)
	y := big.NewInt(2)
	z := big.NewInt(26)
	tx, txErr := c.Decode(TestEnv.DebugContract.EmitInts(c.NewTXOpts(), x, y, z))
	require.NoError(t, txErr, "failed to send transaction")
	require.Equal(t, 1, len(c.Tracer.DecodedCalls), "expected 1 decoded transacton")
	require.NotNil(t, c.Tracer.DecodedCalls[tx.Hash], "expected decoded calls to contain the transaction hash")

	fileName := fmt.Sprintf("traces/%s.json", tx.Hash)
	t.Cleanup(func() {
		_ = os.Remove(fileName)
	})

	expectedCall := &seth.DecodedCall{
		FromAddress: strings.ToLower(c.Addresses[0].Hex()),
		ToAddress:   strings.ToLower(TestEnv.DebugContractAddress.Hex()),
		From:        "you",
		To:          "NetworkDebugContract",
		CommonData: seth.CommonData{
			Signature: "9e099652",
			Method:    "emitInts(int256,int128,uint256)",
			Input:     map[string]interface{}{"first": 1001.0, "second": 2.0, "third": 26.0},
			Output:    map[string]interface{}{"0": 1001.0, "1": 2.0, "2": 26.0},
		},
		Comment: "",
	}

	f, err := os.OpenFile(fileName, os.O_RDONLY, 0666)
	require.NoError(t, err, "expected trace file to exist")

	var readCall []seth.DecodedCall

	defer f.Close()
	b, _ := io.ReadAll(f)
	err = json.Unmarshal(b, &readCall)
	require.NoError(t, err, "failed to unmarshal trace file")

	removeGasDataFromDecodedCalls(map[string][]*seth.DecodedCall{tx.Hash: {&readCall[0]}})

	require.Equal(t, 1, len(readCall), "expected 1 decoded transacton")
	require.EqualValues(t, expectedCall, &readCall[0], "decoded call does not match one read from file")
}

func removeGasDataFromDecodedCalls(decodedCall map[string][]*seth.DecodedCall) {
	for _, decodedCalls := range decodedCall {
		for _, call := range decodedCalls {
			call.GasUsed = 0
			call.GasLimit = 0
		}
	}
}
