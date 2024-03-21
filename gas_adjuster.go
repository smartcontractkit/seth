package seth

import (
	"context"
	"fmt"
	"math"
	"math/big"
	"slices"
	"sync"
	"time"

	"github.com/ethereum/go-ethereum/core/types"
)

const (
	Priority_Ultra    = "ultra"
	Priority_Fast     = "fast"
	Priority_Standard = "standard"
	Priority_Slow     = "slow"

	Congestion_Low    = "low"
	Congestion_Medium = "medium"
	Congestion_High   = "high"
	Congestion_Ultra  = "ultra"
)

const (
	CongestionStrategy_Simple      = "simple"
	CongestionStrategy_NewestFirst = "newest_first"
)

// CalculateNetworkCongestionMetric calculates a simple congestion metric based on the last N blocks
// by averaging the trend in base fee and the gas used ratio.
func (m *Client) CalculateNetworkCongestionMetric(blocksNumber uint64, strategy string) (float64, error) {
	var getHeaderData = func(bn *big.Int) (*types.Header, error) {
		cachedHeader, ok := m.HeaderCache.Get(bn.Int64())
		if ok {
			return cachedHeader, nil
		}

		var timeout uint64 = uint64(blocksNumber / 100)
		if timeout < 2 {
			timeout = 2
		} else if timeout > 5 {
			timeout = 5
		}

		ctx, cancel := context.WithTimeout(context.Background(), time.Duration(timeout)*time.Second)
		defer cancel()
		header, err := m.Client.HeaderByNumber(ctx, bn)
		if err != nil {
			return nil, err
		}
		// ignore the error here as at this points is very improbable that block is nil and there's no error
		_ = m.HeaderCache.Set(header)
		return header, nil
	}

	ctx, cancel := context.WithTimeout(context.Background(), time.Duration(2*time.Second))
	defer cancel()
	lastBlockNumber, err := m.Client.BlockNumber(ctx)
	if err != nil {
		return 0, err
	}

	L.Trace().Msgf("Block range for gas calculation: %d - %d", lastBlockNumber-blocksNumber, lastBlockNumber)

	lastBlock, err := getHeaderData(big.NewInt(int64(lastBlockNumber)))
	if err != nil {
		return 0, err
	}

	var headers []*types.Header
	headers = append(headers, lastBlock)

	channelSize := blocksNumber
	if blocksNumber > 20 {
		channelSize = 20
	}

	var wg sync.WaitGroup
	dataCh := make(chan *types.Header, channelSize)

	go func() {
		for header := range dataCh {
			headers = append(headers, header)
		}
	}()

	startTime := time.Now()
	for i := lastBlockNumber; i > lastBlockNumber-blocksNumber; i-- {
		if i == 1 {
			break
		}

		wg.Add(1)
		go func(bn *big.Int) {
			defer wg.Done()
			header, err := getHeaderData(bn)
			if err != nil {
				L.Error().Err(err).Msgf("Failed to get block %d header", bn.Int64())
				return
			}
			dataCh <- header
		}(big.NewInt(int64(i)))
	}

	wg.Wait()
	close(dataCh)

	endTime := time.Now()
	L.Debug().Msgf("Time to fetch %d block headers: %v", blocksNumber, endTime.Sub(startTime))

	minBlockCount := int(float64(blocksNumber) * 0.8)
	if len(headers) < minBlockCount {
		return 0, fmt.Errorf("Failed to fetch enough block headers for congestion calculation. Wanted at least %d, got %d", minBlockCount, len(headers))
	}

	switch strategy {
	case CongestionStrategy_Simple:
		return calculateSimpleNetworkCongestionMetric(headers), nil
	case CongestionStrategy_NewestFirst:
		return calculateNewestFirstNetworkCongestionMetric(headers), nil
	default:
		return 0, fmt.Errorf("Unknown congestion strategy: %s", strategy)
	}
}

// average the trend and gas used ratio for a basic congestion metric
func calculateSimpleNetworkCongestionMetric(headers []*types.Header) float64 {
	trend := calculateTrend(headers)
	gasUsedRatio := calculateGasUsedRatio(headers)

	congestionMetric := (trend + gasUsedRatio) / 2

	return congestionMetric
}

// calculates a congestion metric using a logarithmic function that gives more weight to most recent block headers
func calculateNewestFirstNetworkCongestionMetric(headers []*types.Header) float64 {
	// sort blocks so that we are sure they are in ascending order
	slices.SortFunc(headers, func(i, j *types.Header) int {
		return int(i.Number.Uint64() - j.Number.Uint64())
	})

	var weightedSum, totalWeight float64
	// Determines how quickly the weight decreases. The lower the number, the higher the weight of newer blocks.
	scaleFactor := 10.0

	// Calculate weights starting from the older to most recent block header.
	for i, header := range headers {
		congestion := float64(header.GasUsed) / float64(header.GasLimit)

		// Applying a logarithmic scale for weights.
		distance := float64(len(headers) - 1 - i)
		weight := 1.0 / math.Log10(distance+scaleFactor)

		weightedSum += congestion * weight
		totalWeight += weight
	}

	if totalWeight == 0 {
		return 0
	}
	return weightedSum / totalWeight
}

// AdjustPriorityFee adjusts the priority fee within a calculated range based on historical data, current congestion, and priority.
func (m *Client) GetSuggestedEIP1559Fees(ctx context.Context, priority string) (maxFeeCap *big.Int, adjustedTipCap *big.Int, err error) {
	L.Info().Msg("Calculating suggested EIP-1559 fees")
	var currentGasTip *big.Int
	currentGasTip, err = m.Client.SuggestGasTipCap(ctx)
	if err != nil {
		return
	}

	L.Debug().
		Str("CurrentGasTip", fmt.Sprintf("%s wei / %s ether", currentGasTip.String(), WeiToEther(currentGasTip).Text('f', -1))).
		Msg("Current suggested gas tip")

	// Fetch the baseline historical base fee and tip for the selected priority
	var baseFee64, historicalSuggestedTip64 float64
	baseFee64, historicalSuggestedTip64, err = m.HistoricalFeeData(priority)
	if err != nil {
		return
	}

	L.Debug().
		Str("HistoricalBaseFee", fmt.Sprintf("%.0f wei / %s ether", baseFee64, WeiToEther(big.NewInt(int64(baseFee64))).Text('f', -1))).
		Str("HistoricalSuggestedTip", fmt.Sprintf("%.0f wei / %s ether", historicalSuggestedTip64, WeiToEther(big.NewInt(int64(historicalSuggestedTip64))).Text('f', -1))).
		Str("Priority", priority).
		Msg("Historical fee data")

	_, tipMagnitudeDiffText := calculateMagnitudeDifference(big.NewFloat(historicalSuggestedTip64), new(big.Float).SetInt(currentGasTip))

	L.Debug().
		Msgf("Historical tip is %s than suggested tip", tipMagnitudeDiffText)

	suggestedGasTip := currentGasTip
	if big.NewInt(int64(historicalSuggestedTip64)).Cmp(suggestedGasTip) > 0 {
		L.Debug().Msg("Historical suggested tip is higher than current suggested tip. Will use it instead.")
		suggestedGasTip = big.NewInt(int64(historicalSuggestedTip64))
	} else {
		L.Debug().Msg("Suggested tip is higher than historical tip. Will use suggested tip.")
	}

	if m.Cfg.IsExperimentEnabled(Experiment_Eip1559FeeEqualier) {
		L.Debug().Msg("FeeEqualier experiment is enabled. Will adjust base fee and tip to be of the same order of magnitude.")
		baseFeeTipMagnitudeDiff, _ := calculateMagnitudeDifference(big.NewFloat(baseFee64), new(big.Float).SetInt(suggestedGasTip))

		//one of values is 0, inifite order of magnitude smaller or larger
		if baseFeeTipMagnitudeDiff == -0 {
			if baseFee64 == 0.0 {
				L.Debug().Msg("Historical base fee is 0.0. Will use suggested tip as base fee.")
				baseFee64 = float64(suggestedGasTip.Int64())
			} else {
				L.Debug().Msg("Suggested tip is 0.0. Will use historical base fee as tip.")
				suggestedGasTip = big.NewInt(int64(baseFee64))
			}
		} else if baseFeeTipMagnitudeDiff < 3 {
			L.Debug().Msg("Historical base fee is 3 orders of magnitude lower than suggested tip. Will use suggested tip as base fee.")
			baseFee64 = float64(suggestedGasTip.Int64())
		} else if baseFeeTipMagnitudeDiff > 3 {
			L.Debug().Msg("Suggested tip is 3 orders of magnitude lower than historical base fee. Will use historical base fee as tip.")
			suggestedGasTip = big.NewInt(int64(baseFee64))
		}
	}

	// Adjust the suggestedTip based on current congestion, keeping within reasonable bounds
	var adjustmentFactor float64
	adjustmentFactor, err = getAdjustmentFactor(priority)
	if err != nil {
		return
	}

	var congestionMetric float64
	congestionMetric, err = m.CalculateNetworkCongestionMetric(m.Cfg.Network.GasEstimationBlocks, CongestionStrategy_NewestFirst)
	if err != nil {
		return
	}

	congestionClassificaion := classifyCongestion(congestionMetric)

	L.Debug().
		Str("CongestionMetric", fmt.Sprintf("%.4f", congestionMetric)).
		Str("CongestionClassificaion", congestionClassificaion).
		Msg("Calculated congestion metric")

	// Calculate adjusted tip based on congestion and priority
	congestionAdjustment := new(big.Float).Mul(big.NewFloat(congestionMetric*adjustmentFactor), new(big.Float).SetFloat64(float64(suggestedGasTip.Int64())))
	congestionAdjustmentInt, _ := congestionAdjustment.Int(nil)

	adjustedTipCap = new(big.Int).Add(suggestedGasTip, congestionAdjustmentInt)
	adjustedBaseFee := new(big.Int).Add(big.NewInt(int64(baseFee64)), congestionAdjustmentInt)

	// Calculate the base max fee (without buffer) as initialBaseFee + finalTip.
	rawMaxFeeCap := new(big.Int).Add(adjustedBaseFee, adjustedTipCap)

	// Adjust the max fee based on the base fee, tip, and congestion-based buffer.
	var bufferPercent float64
	bufferPercent, err = getBufferPercent(congestionClassificaion)
	if err != nil {
		return
	}

	// Calculate and apply the buffer.
	buffer := new(big.Float).Mul(new(big.Float).SetInt(rawMaxFeeCap), big.NewFloat(bufferPercent))
	bufferInt, _ := buffer.Int(nil)
	maxProposedFeeCap := new(big.Int).Add(rawMaxFeeCap, bufferInt)
	maxFeeCap = maxProposedFeeCap

	maxAllowedTxCost := big.NewInt(m.Cfg.Network.GasEstimationMaxTxCostWei)
	maxPossibleTxCost := big.NewInt(0).Mul(maxProposedFeeCap, big.NewInt(int64(m.Cfg.Network.GasLimit)))
	gasTipDiffText := "none"
	gasCapDiffText := "none"
	totalCostDiffText := "none"

	if maxPossibleTxCost.Cmp(maxAllowedTxCost) > 0 {
		_, txCostMagnitudeDiffText := calculateMagnitudeDifference(new(big.Float).SetInt(maxPossibleTxCost), new(big.Float).SetInt(maxAllowedTxCost))
		L.Debug().
			Str("Overflow", fmt.Sprintf("%s wei / %s ether", big.NewInt(0).Sub(maxPossibleTxCost, maxAllowedTxCost).String(), WeiToEther(big.NewInt(0).Sub(maxPossibleTxCost, maxAllowedTxCost)).Text('f', -1))).
			Msgf("Max possible tx cost is %s than allowed tx cost and exceeds it. Will cap it.", txCostMagnitudeDiffText)

		maxFeeCap = big.NewInt(0).Div(maxAllowedTxCost, big.NewInt(int64(m.Cfg.Network.GasLimit)))
		changeRatio := big.NewFloat(0).Quo(new(big.Float).SetInt(maxFeeCap), new(big.Float).SetInt(rawMaxFeeCap))

		newAdjustedTipCap, _ := new(big.Float).Mul(new(big.Float).SetInt(adjustedTipCap), changeRatio).Int64()
		newAdjustedTipCapBigInt := big.NewInt(newAdjustedTipCap)

		L.Debug().
			Str("Change", fmt.Sprintf("%s wei / %s ether", big.NewInt(0).Sub(adjustedTipCap, newAdjustedTipCapBigInt).String(), WeiToEther(big.NewInt(0).Sub(adjustedTipCap, newAdjustedTipCapBigInt)).Text('f', -1))).
			Msg("Proportionally decreasing tip to fit within max allowed tx cost.")

		adjustedTipCap = newAdjustedTipCapBigInt
		gasTipDiffText = fmt.Sprintf("%s wei / %s ether", big.NewInt(0).Sub(adjustedTipCap, suggestedGasTip).String(), WeiToEther(big.NewInt(0).Sub(adjustedTipCap, suggestedGasTip)).Text('f', -1))
		totalCostDiffText = fmt.Sprintf("%s wei / %s ether", big.NewInt(0).Sub(maxPossibleTxCost, maxAllowedTxCost).String(), WeiToEther(big.NewInt(0).Sub(maxPossibleTxCost, maxAllowedTxCost)).Text('f', -1))
		gasCapDiffText = fmt.Sprintf("%s wei / %s ether", big.NewInt(0).Sub(maxProposedFeeCap, maxFeeCap).String(), WeiToEther(big.NewInt(0).Sub(maxProposedFeeCap, maxFeeCap)).Text('f', -1))
	}

	L.Debug().
		Str("Diff", gasTipDiffText).
		Str("Initial GasTipCap", fmt.Sprintf("%s wei / %s ether", suggestedGasTip.String(), WeiToEther(suggestedGasTip).Text('f', -1))).
		Str("Final GasTipCap", fmt.Sprintf("%s wei / %s ether", adjustedTipCap.String(), WeiToEther(adjustedTipCap).Text('f', -1))).
		Msg("Tip Cap adjustment")

	L.Debug().
		Str("Diff", gasCapDiffText).
		Str("Initial Fee Cap", fmt.Sprintf("%s wei / %s ether", maxProposedFeeCap.String(), WeiToEther(maxProposedFeeCap).Text('f', -1))).
		Str("Final Fee Cap", fmt.Sprintf("%s wei / %s ether", maxFeeCap.String(), WeiToEther(maxFeeCap).Text('f', -1))).
		Msg("Fee Cap adjustment")

	L.Debug().
		Str("Diff", totalCostDiffText).
		Str("MaxAllowedTxCost", fmt.Sprintf("%s wei / %s ether", maxAllowedTxCost.String(), WeiToEther(maxAllowedTxCost).Text('f', -1))).
		Str("MaxPossibleTxCost", fmt.Sprintf("%s wei / %s ether", maxPossibleTxCost.String(), WeiToEther(maxPossibleTxCost).Text('f', -1))).
		Msg("Tx cost adjustment")

	L.Debug().
		Str("CongestionMetric", fmt.Sprintf("%.4f", congestionMetric)).
		Str("CongestionClassificaion", congestionClassificaion).
		Float64("AdjustmentFactor", adjustmentFactor).
		Str("Priority", priority).
		Msg("Adjustment factors")

	L.Info().
		Str("GasTipCap", fmt.Sprintf("%s wei / %s ether", adjustedTipCap.String(), WeiToEther(adjustedTipCap).Text('f', -1))).
		Str("GasFeeCap", fmt.Sprintf("%s wei / %s ether", maxFeeCap.String(), WeiToEther(maxFeeCap).Text('f', -1))).
		Msg("Calculated suggested EIP-1559 fees")

	return
}

// GetSuggestedLegacyFees calculates the suggested gas price based on historical data, current congestion, and priority.
func (m *Client) GetSuggestedLegacyFees(ctx context.Context, priority string) (adjustedGasPrice *big.Int, err error) {
	L.Info().
		Msg("Calculating suggested Legacy fees")

	var suggestedGasPrice *big.Int
	suggestedGasPrice, err = m.Client.SuggestGasPrice(ctx)
	if err != nil {
		return
	}

	// Adjust the suggestedTip based on current congestion, keeping within reasonable bounds
	var adjustmentFactor float64
	adjustmentFactor, err = getAdjustmentFactor(priority)
	if err != nil {
		return
	}

	var congestionMetric float64
	congestionMetric, err = m.CalculateNetworkCongestionMetric(m.Cfg.Network.GasEstimationBlocks, CongestionStrategy_NewestFirst)
	if err != nil {
		return
	}

	congestionClassificaion := classifyCongestion(congestionMetric)

	L.Debug().
		Str("CongestionMetric", fmt.Sprintf("%.4f", congestionMetric)).
		Str("CongestionClassificaion", congestionClassificaion).
		Msg("Calculated congestion metric")

	// Calculate adjusted tip based on congestion and priority
	congestionAdjustment := new(big.Float).Mul(big.NewFloat(congestionMetric*adjustmentFactor), new(big.Float).SetFloat64(float64(suggestedGasPrice.Int64())))
	congestionAdjustmentInt, _ := congestionAdjustment.Int(nil)

	adjustedGasPrice = new(big.Int).Add(suggestedGasPrice, congestionAdjustmentInt)

	// Adjust the max fee based on the base fee, tip, and congestion-based buffer.
	var bufferPercent float64
	bufferPercent, err = getBufferPercent(congestionClassificaion)
	if err != nil {
		return
	}

	// Calculate and apply the buffer.
	buffer := new(big.Float).Mul(new(big.Float).SetInt(adjustedGasPrice), big.NewFloat(bufferPercent))
	bufferInt, _ := buffer.Int(nil)
	adjustedGasPrice = new(big.Int).Add(adjustedGasPrice, bufferInt)

	maxAllowedTxCost := big.NewInt(m.Cfg.Network.GasEstimationMaxTxCostWei)
	maxPossibleTxCost := big.NewInt(0).Mul(adjustedGasPrice, big.NewInt(int64(m.Cfg.Network.GasLimit)))

	// Ensure the adjusted gas price does not exceed the max gas price
	if maxPossibleTxCost.Cmp(maxAllowedTxCost) > 0 {
		L.Debug().
			Str("Overflow (Wei/Ether)", fmt.Sprintf("%s/%s", big.NewInt(0).Sub(maxPossibleTxCost, maxAllowedTxCost).String(), WeiToEther(big.NewInt(0).Sub(maxPossibleTxCost, maxAllowedTxCost)))).
			Msg("Max possible tx cost exceeds max allowed tx cost. Capping it.")

		newAdjustedGasPrice := big.NewInt(0).Div(maxAllowedTxCost, big.NewInt(int64(m.Cfg.Network.GasLimit)))

		L.Debug().
			Str("Change (Wei/Ether)", fmt.Sprintf("%s/%s", big.NewInt(0).Sub(adjustedGasPrice, newAdjustedGasPrice).String(), WeiToEther(big.NewInt(0).Sub(adjustedGasPrice, newAdjustedGasPrice)))).
			Msg("Decreasing gas price to fit within max allowed tx cost.")

		adjustedGasPrice = newAdjustedGasPrice
	}

	L.Debug().
		Str("Diff (Wei/Ether)", fmt.Sprintf("%s/%s", big.NewInt(0).Sub(adjustedGasPrice, suggestedGasPrice).String(), WeiToEther(big.NewInt(0).Sub(adjustedGasPrice, suggestedGasPrice)))).
		Str("Initial GasPrice (Wei/Ether)", fmt.Sprintf("%s/%s", suggestedGasPrice.String(), WeiToEther(suggestedGasPrice))).
		Str("Final GasPrice (Wei/Ether)", fmt.Sprintf("%s/%s", adjustedGasPrice.String(), WeiToEther(adjustedGasPrice))).
		Msg("Suggested Legacy fees")

	L.Debug().
		Str("CongestionMetric", fmt.Sprintf("%.4f", congestionMetric)).
		Str("CongestionClassificaion", congestionClassificaion).
		Float64("AdjustmentFactor", adjustmentFactor).
		Str("Priority", priority).
		Msg("Suggested Legacy fees")

	L.Info().
		Str("GasPrice", fmt.Sprintf("%s wei / %s ether", adjustedGasPrice.String(), WeiToEther(adjustedGasPrice).Text('f', -1))).
		Msg("Calculated suggested Legacy fees")

	return
}

func getAdjustmentFactor(priority string) (float64, error) {
	switch priority {
	case Priority_Ultra:
		return 1.5, nil
	case Priority_Fast:
		return 1.2, nil
	case Priority_Standard:
		return 1.0, nil
	case Priority_Slow:
		return 0.8, nil
	default:
		return 0, fmt.Errorf("Unknown priority: %s", priority)
	}
}

func getBufferPercent(congestionClassification string) (float64, error) {
	switch congestionClassification {
	case Congestion_Low:
		return 0.05, nil
	case Congestion_Medium:
		return 0.10, nil
	case Congestion_High:
		return 0.15, nil
	case Congestion_Ultra:
		return 0.20, nil
	default:
		return 0, fmt.Errorf("Unknown congestion classification: %s", congestionClassification)
	}
}

func classifyCongestion(congestionMetric float64) string {
	switch {
	case congestionMetric < 0.33:
		return Congestion_Low
	case congestionMetric <= 0.66:
		return Congestion_Medium
	case congestionMetric <= 0.75:
		return Congestion_High
	default:
		return Congestion_Ultra
	}
}

func (m *Client) HistoricalFeeData(priority string) (baseFee float64, historicalGasTipCap float64, err error) {
	estimator := NewGasEstimator(m)
	stats, err := estimator.Stats(m.Cfg.Network.GasEstimationBlocks, 99)
	if err != nil {
		L.Error().
			Err(err).
			Msg("Failed to get fee history. Skipping automation gas estimation")

		return
	} else {
		switch priority {
		case Priority_Ultra:
			baseFee = stats.GasPrice.Max
			historicalGasTipCap = stats.TipCap.Max
		case Priority_Fast:
			baseFee = stats.GasPrice.Perc99
			historicalGasTipCap = stats.TipCap.Perc99
		case Priority_Standard:
			baseFee = stats.GasPrice.Perc50
			historicalGasTipCap = stats.TipCap.Perc50
		case Priority_Slow:
			baseFee = stats.GasPrice.Perc25
			historicalGasTipCap = stats.TipCap.Perc25
		default:
			L.Error().
				Str("Priority", priority).
				Msg("Unknown priority. Skipping automation gas estimation")
			m.Errors = append(m.Errors, err)
		}
	}

	return baseFee, historicalGasTipCap, err
}

// CalculateTrend analyzes the change in base fee to determine congestion trend
func calculateTrend(headers []*types.Header) float64 {
	var totalIncrease float64
	for i := 1; i < len(headers); i++ {
		if headers[i].BaseFee.Cmp(headers[i-1].BaseFee) > 0 {
			totalIncrease += 1
		}
	}
	// Normalize the increase by the number of transitions to get an average trend
	trend := totalIncrease / float64(len(headers)-1)
	return trend
}

// CalculateGasUsedRatio averages the gas used ratio for a sense of how full blocks are
func calculateGasUsedRatio(headers []*types.Header) float64 {
	var totalRatio float64
	for _, header := range headers {
		ratio := float64(header.GasUsed) / float64(header.GasLimit)
		totalRatio += ratio
	}
	averageRatio := totalRatio / float64(len(headers))
	return averageRatio
}

func calculateMagnitudeDifference(first, second *big.Float) (int, string) {
	firstFloat, _ := first.Float64()
	secondFloat, _ := second.Float64()

	if firstFloat == 0.0 {
		return -0, "infinite orders of magnitude smaller"
	}

	if secondFloat == 0.0 {
		return -0, "infinite orders of magnitude larger"
	}

	firstOrderOfMagnitude := math.Log10(firstFloat)
	secondOrderOfMagnitude := math.Log10(secondFloat)

	diff := firstOrderOfMagnitude - secondOrderOfMagnitude

	if diff < 0 {
		intDiff := math.Floor(diff)
		return int(intDiff), fmt.Sprintf("%d orders of magnitude smaller", int(math.Abs(intDiff)))
	} else if diff > 0 && diff <= 1 {
		return 0, "the same order of magnitude"
	}

	intDiff := int(math.Ceil(diff))
	return intDiff, fmt.Sprintf("%d orders of magnitude larger", intDiff)
}
