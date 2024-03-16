package seth

import (
	"fmt"
	"github.com/ethereum/go-ethereum/accounts/abi"
	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/common/hexutil"
	"github.com/ethereum/go-ethereum/rpc"
	"github.com/pkg/errors"
	"github.com/rs/zerolog"
	"strconv"
	"strings"
)

const (
	ErrNoTrace                   = "no trace found"
	ErrNoABIMethod               = "no ABI method found"
	ErrNoAbiFound                = "no ABI found in Contract Store"
	ErrNoFourByteFound           = "no method signatures found in tracing data"
	ErrInvalidMethodSignature    = "no method signature found or it's not 4 bytes long"
	ErrSignatureNotFoundIn4Bytes = "signature not found in 4 bytes trace"
	WrnMissingCallTrace          = "This call was missing from call trace, but it's signature was present in 4bytes trace. Most data is missing; Call order remains unknown"

	FAILED_TO_DECODE = "failed to decode"
	UNKNOWN          = "unknown"
	NO_DATA          = "no data"

	CommentMissingABI = "Call not decoded due to missing ABI instance"
)

type Tracer struct {
	Cfg                      *Config
	rpcClient                *rpc.Client
	traces                   map[string]*Trace
	Addresses                []common.Address
	ContractStore            *ContractStore
	ContractAddressToNameMap ContractMap
	DecodedCalls             map[string][]*DecodedCall
	ABIFinder                *ABIFinder
}

type ContractMap map[string]string

func (c ContractMap) IsKnownAddress(addr string) bool {
	return c[strings.ToLower(addr)] != ""
}

func (c ContractMap) GetContractName(addr string) string {
	return c[strings.ToLower(addr)]
}

func (c ContractMap) GetContractAddress(addr string) string {
	if addr == UNKNOWN {
		return UNKNOWN
	}

	for k, v := range c {
		if v == addr {
			return k
		}
	}
	return UNKNOWN
}

func (c ContractMap) AddContract(addr, name string) {
	if addr == UNKNOWN {
		return
	}

	name = strings.TrimSuffix(name, ".abi")
	c[strings.ToLower(addr)] = name
}

type Trace struct {
	TxHash       string
	FourByte     map[string]*TXFourByteMetadataOutput
	CallTrace    *TXCallTraceOutput
	OpCodesTrace map[string]interface{}
}

type TXFourByteMetadataOutput struct {
	CallSize int
	Times    int
}

type TXCallTraceOutput struct {
	Call
	Calls []Call `json:"calls"`
}

func (t *TXCallTraceOutput) AsCall() Call {
	return t.Call
}

type TraceLog struct {
	Address string   `json:"address"`
	Data    string   `json:"data"`
	Topics  []string `json:"topics"`
}

func (t TraceLog) GetTopics() []common.Hash {
	var h []common.Hash
	for _, v := range t.Topics {
		h = append(h, common.HexToHash(v))
	}
	return h
}

func (t TraceLog) GetData() []byte {
	return common.Hex2Bytes(strings.TrimPrefix(t.Data, "0x"))
}

type Call struct {
	From    string     `json:"from"`
	Gas     string     `json:"gas"`
	GasUsed string     `json:"gasUsed"`
	Input   string     `json:"input"`
	Logs    []TraceLog `json:"logs"`
	Output  string     `json:"output"`
	To      string     `json:"to"`
	Type    string     `json:"type"`
	Value   string     `json:"value"`
}

func NewTracer(url string, cs *ContractStore, abiFinder *ABIFinder, cfg *Config, contractAddressToNameMap map[string]string, addresses []common.Address) (*Tracer, error) {
	c, err := rpc.Dial(url)
	if err != nil {
		return nil, fmt.Errorf("failed to connect to '%s' due to: %w", url, err)
	}
	return &Tracer{
		Cfg:                      cfg,
		rpcClient:                c,
		traces:                   make(map[string]*Trace),
		Addresses:                addresses,
		ContractStore:            cs,
		ContractAddressToNameMap: contractAddressToNameMap,
		DecodedCalls:             make(map[string][]*DecodedCall),
		ABIFinder:                abiFinder,
	}, nil
}

func (t *Tracer) TraceGethTX(txHash string) error {
	fourByte, err := t.trace4Byte(txHash)
	if err != nil {
		return err
	}
	callTrace, err := t.traceCallTracer(txHash)
	if err != nil {
		return err
	}

	opCodesTrace, err := t.traceOpCodesTracer(txHash)
	if err != nil {
		return err
	}
	t.traces[txHash] = &Trace{
		TxHash:       txHash,
		FourByte:     fourByte,
		CallTrace:    callTrace,
		OpCodesTrace: opCodesTrace,
	}
	_, err = t.DecodeTrace(L, *t.traces[txHash])
	if err != nil {
		return err
	}
	return t.PrintTXTrace(txHash)
}

func (t *Tracer) PrintTXTrace(txHash string) error {
	trace, ok := t.traces[txHash]
	if !ok {
		return errors.New(ErrNoTrace)
	}
	l := L.With().Str("Transaction", txHash).Logger()
	l.Debug().Interface("4Byte", trace.FourByte).Msg("Calls function signatures (names)")
	l.Debug().Interface("CallTrace", trace.CallTrace).Msg("Full call trace with logs")
	return nil
}

func (t *Tracer) trace4Byte(txHash string) (map[string]*TXFourByteMetadataOutput, error) {
	var trace map[string]int
	if err := t.rpcClient.Call(&trace, "debug_traceTransaction", txHash, map[string]interface{}{"tracer": "4byteTracer"}); err != nil {
		return nil, err
	}
	out := make(map[string]*TXFourByteMetadataOutput)
	for k, v := range trace {
		d := strings.Split(k, "-")
		callParamsSize, err := strconv.Atoi(d[1])
		if err != nil {
			return nil, err
		}
		out[d[0]] = &TXFourByteMetadataOutput{Times: v, CallSize: callParamsSize}
	}
	return out, nil
}

func (t *Tracer) traceCallTracer(txHash string) (*TXCallTraceOutput, error) {
	var trace *TXCallTraceOutput
	if err := t.rpcClient.Call(
		&trace,
		"debug_traceTransaction",
		txHash,
		map[string]interface{}{
			"tracer": "callTracer",
			"tracerConfig": map[string]interface{}{
				"withLog": true,
			},
		}); err != nil {
		return nil, err
	}
	return trace, nil
}

func (t *Tracer) traceOpCodesTracer(txHash string) (map[string]interface{}, error) {
	var trace map[string]interface{}
	if err := t.rpcClient.Call(&trace, "debug_traceTransaction", txHash); err != nil {
		return nil, err
	}
	return trace, nil
}

// DecodeTrace decodes the trace of a transaction including all subcalls. It returns a list of decoded calls.
// Depending on the config it also saves the decoded calls as JSON files.
func (t *Tracer) DecodeTrace(l zerolog.Logger, trace Trace) ([]*DecodedCall, error) {
	decodedCalls := []*DecodedCall{}

	if t.ContractStore == nil {
		L.Warn().Msg(WarnNoContractStore)
		return []*DecodedCall{}, nil
	}

	// we can still decode the calls without 4byte signatures
	if len(trace.FourByte) == 0 {
		L.Warn().Msg(ErrNoFourByteFound)
	}

	methods := make([]string, 0, len(trace.CallTrace.Calls)+1)

	var getSignature = func(input string) (string, error) {
		if len(input) < 10 {
			err := errors.New(ErrInvalidMethodSignature)
			l.Err(err).
				Str("Input", input).
				Send()
			return "", errors.New(ErrInvalidMethodSignature)
		}

		return input[2:10], nil
	}

	mainSig, err := getSignature(trace.CallTrace.Input)
	if err != nil {
		return nil, err
	}
	methods = append(methods, mainSig)

	for _, call := range trace.CallTrace.Calls {
		sig, err := getSignature(call.Input)
		if err != nil {
			return nil, err
		}

		methods = append(methods, sig)
	}

	decodedMainCall, err := t.decodeCall(common.Hex2Bytes(methods[0]), trace.CallTrace.AsCall())
	if err != nil {
		l.Debug().
			Err(err).
			Str("From", decodedMainCall.FromAddress).
			Str("To", decodedMainCall.ToAddress).
			Msg("Failed to decode main call")

		return nil, err
	}

	decodedCalls = append(decodedCalls, decodedMainCall)

	for i, call := range trace.CallTrace.Calls {
		method := common.Hex2Bytes(methods[i+1])
		decodedSubCall, err := t.decodeCall(method, call)
		if err != nil {
			l.Debug().
				Err(err).
				Str("From", call.From).
				Str("To", call.To).
				Msg("Failed to decode sub call")
			decodedCalls = append(decodedCalls, &DecodedCall{
				CommonData: CommonData{Method: FAILED_TO_DECODE,
					Input:  map[string]interface{}{"error": FAILED_TO_DECODE},
					Output: map[string]interface{}{"error": FAILED_TO_DECODE},
				},
				FromAddress: call.From,
				ToAddress:   call.To,
			})
			continue
		}
		decodedCalls = append(decodedCalls, decodedSubCall)
	}

	missingCalls := t.checkForMissingCalls(trace)
	decodedCalls = append(decodedCalls, missingCalls...)

	if len(decodedCalls) != 0 {
		l.Debug().
			Msg("----------- Decoding transaction trace started -----------")
		for _, decodedCall := range decodedCalls {
			t.printDecodedCallData(l, decodedCall)
		}
		l.Debug().
			Msg("----------- Decoding transaction trace finished -----------")
	}

	t.DecodedCalls[trace.TxHash] = decodedCalls

	if t.Cfg.TraceToJson {
		saveErr := t.SaveDecodedCallsAsJson("traces")
		if saveErr != nil {
			L.Warn().
				Err(saveErr).
				Msg("Failed to save decoded calls as JSON")
		}
	}

	return decodedCalls, nil
}

func (t *Tracer) decodeCall(byteSignature []byte, rawCall Call) (*DecodedCall, error) {
	var txInput map[string]interface{}
	var txOutput map[string]interface{}
	var txEvents []DecodedCommonLog

	var generateDuplicatesComment = func(abiResult ABIFinderResult) string {
		var comment string
		if abiResult.DuplicateCount > 0 {
			comment = fmt.Sprintf("potentially inaccurate - method present in %d other contracts", abiResult.DuplicateCount)
		}

		return comment
	}

	defaultCall := getDefaultDecodedCall()

	abiResult, err := t.ABIFinder.FindABIByMethod(rawCall.To, byteSignature)

	defaultCall.CommonData.Signature = common.Bytes2Hex(byteSignature)
	defaultCall.FromAddress = rawCall.From
	defaultCall.ToAddress = rawCall.To
	defaultCall.From = t.getHumanReadableAddressName(rawCall.From)
	defaultCall.To = t.getHumanReadableAddressName(rawCall.To) //somehow mark it with "*"
	defaultCall.Comment = generateDuplicatesComment(abiResult)

	if rawCall.Value != "0x0" {
		decimalValue, err := strconv.ParseInt(strings.TrimPrefix(rawCall.Value, "0x"), 16, 64)
		if err != nil {
			L.Warn().
				Err(err).
				Str("Value", rawCall.Value).
				Msg("Failed to parse value")
		} else {
			defaultCall.Value = decimalValue
		}
	}

	if rawCall.Gas != "0x0" {
		decimalValue, err := strconv.ParseInt(strings.TrimPrefix(rawCall.Gas, "0x"), 16, 64)
		if err != nil {
			L.Warn().
				Err(err).
				Str("Gas", rawCall.Gas).
				Msg("Failed to parse value")
		} else {
			defaultCall.GasLimit = uint64(decimalValue)
		}
	}

	if rawCall.GasUsed != "0x0" {
		decimalValue, err := strconv.ParseInt(strings.TrimPrefix(rawCall.GasUsed, "0x"), 16, 64)
		if err != nil {
			L.Warn().
				Err(err).
				Str("GasUsed", rawCall.GasUsed).
				Msg("Failed to parse value")
		} else {
			defaultCall.GasUsed = uint64(decimalValue)
		}
	}

	if err != nil {
		if defaultCall.Comment != "" {
			defaultCall.Comment = fmt.Sprintf("%s; %s", defaultCall.Comment, CommentMissingABI)
		} else {
			defaultCall.Comment = CommentMissingABI
		}
		L.Warn().
			Err(err).
			Str("Method signature", common.Bytes2Hex(byteSignature)).
			Str("Contract", rawCall.To).
			Msg("Method not found in any ABI instance. Unable to provide full tracing information")

		// let's not return the error, as we can still provide some information
		return defaultCall, nil
	}

	defaultCall.Method = abiResult.Method.Sig
	defaultCall.Signature = common.Bytes2Hex(abiResult.Method.ID)

	txInput, err = decodeTxInputs(L, common.Hex2Bytes(strings.TrimPrefix(rawCall.Input, "0x")), abiResult.Method)
	if err != nil {
		return defaultCall, errors.Wrap(err, ErrDecodeInput)
	}

	defaultCall.Input = txInput

	if rawCall.Output != "" {
		output, err := hexutil.Decode(rawCall.Output)
		if err != nil {
			return defaultCall, errors.Wrap(err, ErrDecodeOutput)
		}
		txOutput, err = decodeTxOutputs(L, output, abiResult.Method)
		if err != nil {
			return defaultCall, errors.Wrap(err, ErrDecodeOutput)
		}

		defaultCall.Output = txOutput
	}

	txEvents, err = t.decodeContractLogs(L, rawCall.Logs, abiResult.ABI)
	if err != nil {
		return defaultCall, err
	}

	defaultCall.Events = txEvents

	return defaultCall, nil
}

func (t *Tracer) isOwnAddress(addr string) bool {
	for _, a := range t.Addresses {
		if strings.ToLower(a.Hex()) == addr {
			return true
		}
	}

	return false
}

func (t *Tracer) checkForMissingCalls(trace Trace) []*DecodedCall {
	expected := 0
	for _, v := range trace.FourByte {
		expected += v.Times
	}

	diff := expected - (len(trace.CallTrace.Calls) + 1)
	if diff != 0 {
		L.Debug().
			Int("Debugged calls", len(trace.CallTrace.Calls)+1).
			Int("4byte signatures", len(trace.FourByte)).
			Msgf("Number of calls and signatures does not match. There were %d more call that were't debugged", diff)

		unknownCall := &DecodedCall{
			CommonData: CommonData{Method: NO_DATA,
				Input:  map[string]interface{}{"warning": NO_DATA},
				Output: map[string]interface{}{"warning": NO_DATA},
			},
			FromAddress: UNKNOWN,
			ToAddress:   UNKNOWN,
			Events: []DecodedCommonLog{
				{Signature: NO_DATA, EventData: map[string]interface{}{"warning": NO_DATA}},
			},
		}

		missingSignatures := []string{}
		for k := range trace.FourByte {
			found := false
			for _, call := range trace.CallTrace.Calls {
				if strings.Contains(call.Input, k) {
					found = true
					break
				}
			}

			if strings.Contains(trace.CallTrace.Input, k) {
				found = true
			}

			if !found {
				missingSignatures = append(missingSignatures, k)
			}
		}

		missedCalls := make([]*DecodedCall, 0, len(missingSignatures))

		for _, missingSig := range missingSignatures {
			byteSignature := common.Hex2Bytes(strings.TrimPrefix(missingSig, "0x"))
			humanName := missingSig

			abiResult, err := t.ABIFinder.FindABIByMethod(UNKNOWN, byteSignature)
			if err != nil {
				L.Info().
					Str("Signature", humanName).
					Msg("Method not found in any ABI instance. Unable to provide any more tracing information")

				missedCalls = append(missedCalls, unknownCall)
			}

			toAddress := t.ContractAddressToNameMap.GetContractAddress(abiResult.ContractName())
			comment := WrnMissingCallTrace
			if abiResult.DuplicateCount > 0 {
				comment = fmt.Sprintf("%s; Potentially inaccurate - method present in %d other contracts", comment, abiResult.DuplicateCount)
			}

			missedCalls = append(missedCalls, &DecodedCall{
				CommonData: CommonData{
					Signature: humanName,
					Method:    abiResult.Method.Name,
					Input:     map[string]interface{}{"warning": NO_DATA},
					Output:    map[string]interface{}{"warning": NO_DATA},
				},
				FromAddress: UNKNOWN,
				ToAddress:   toAddress,
				To:          abiResult.ContractName(),
				From:        UNKNOWN,
				Comment:     comment,
				Events: []DecodedCommonLog{
					{Signature: NO_DATA, EventData: map[string]interface{}{"warning": NO_DATA}},
				},
			})
		}

		return missedCalls
	}

	return []*DecodedCall{}
}

func (t *Tracer) SaveDecodedCallsAsJson(dirname string) error {
	for txHash, calls := range t.DecodedCalls {
		_, err := saveAsJson(calls, dirname, txHash)
		if err != nil {
			return err
		}
	}

	return nil
}

func (t *Tracer) decodeContractLogs(l zerolog.Logger, logs []TraceLog, a abi.ABI) ([]DecodedCommonLog, error) {
	l.Trace().Msg("Decoding events")
	var eventsParsed []DecodedCommonLog
	for _, lo := range logs {
		for _, evSpec := range a.Events {
			if evSpec.ID.Hex() == lo.Topics[0] {
				l.Trace().Str("Name", evSpec.RawName).Str("Signature", evSpec.Sig).Msg("Unpacking event")
				eventsMap, topicsMap, err := decodeEventFromLog(l, a, evSpec, lo)
				if err != nil {
					return nil, errors.Wrap(err, ErrDecodeLog)
				}
				parsedEvent := decodedLogFromMaps(&DecodedCommonLog{}, eventsMap, topicsMap)
				if decodedLog, ok := parsedEvent.(*DecodedCommonLog); ok {
					decodedLog.Signature = evSpec.Sig
					t.mergeLogMeta(decodedLog, lo)
					eventsParsed = append(eventsParsed, *decodedLog)
					l.Trace().Interface("Log", parsedEvent).Msg("Transaction log")
				} else {
					l.Trace().
						Str("Actual type", fmt.Sprintf("%T", decodedLog)).
						Msg("Failed to cast decoded event to DecodedCommonLog")
				}
			}
		}
	}
	return eventsParsed, nil
}

// mergeLogMeta add metadata from log
func (t *Tracer) mergeLogMeta(pe *DecodedCommonLog, l TraceLog) {
	pe.Address = common.HexToAddress(l.Address)
	pe.Topics = l.Topics
}

func (t *Tracer) getHumanReadableAddressName(address string) string {
	if t.ContractAddressToNameMap.IsKnownAddress(address) {
		address = t.ContractAddressToNameMap.GetContractName(address)
	} else if t.isOwnAddress(address) {
		address = "you"
	} else {
		address = "unknown"
	}

	return address
}

// printDecodedCallData prints decoded txn data
func (t *Tracer) printDecodedCallData(l zerolog.Logger, dc *DecodedCall) {
	l.Debug().Str("Call", fmt.Sprintf("%s -> %s", dc.FromAddress, dc.ToAddress)).Send()
	l.Debug().Str("Call", fmt.Sprintf("%s -> %s", dc.From, dc.To)).Send()

	l.Debug().Str("Method signature", dc.Signature).Send()
	l.Debug().Str("Method name", dc.Method).Send()
	l.Debug().Str("Gas used/limit", fmt.Sprintf("%d/%d", dc.GasUsed, dc.GasLimit)).Send()
	l.Debug().Str("Gas left", fmt.Sprintf("%d", dc.GasLimit-dc.GasUsed)).Send()
	if dc.Comment != "" {
		l.Debug().Str("Comment", dc.Comment).Send()
	}
	if dc.Input != nil {
		l.Debug().Interface("Inputs", dc.Input).Send()
	}
	if dc.Output != nil {
		l.Debug().Interface("Outputs", dc.Output).Send()
	}
	for _, e := range dc.Events {
		l.Debug().
			Str("Signature", e.Signature).
			Interface("Log", e.EventData).Send()
	}
}
