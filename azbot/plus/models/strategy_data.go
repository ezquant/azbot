package models

import (
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"sync"
	"time"

	"github.com/go-resty/resty/v2"
)

const (
	USDSymbol       = "USDT"
	coinNotFoundErr = "coin symbol \"%s\" not found"
	noCoinsListErr  = "no coins list in the JSON response"
)

var coningeckoSlugs = map[string]string{
	"ADA":  "cardano",
	"AVAX": "avalanche-2",
	"BNB":  "binancecoin",
	"BTC":  "bitcoin",
	"DOGE": "dogecoin",
	"DOT":  "polkadot",
	"ETH":  "ethereum",
	"HBAR": "hedera-hashgraph",
	"LINK": "chainlink",
	"SOL":  "solana",
	"SUI":  "sui",
	"TON":  "the-open-network",
	"TRX":  "tron",
	"UNI":  "uniswap",
	"XLM":  "stellar",
	"XRP":  "ripple",
}

var (
	cacheMutex sync.Mutex
	cacheData  []map[string]string
	cacheTime  time.Time
)

type StrategyData struct {
	//MinimumBalance    float64 // for DCAOnSteroids
	//ExpectedPriceDrop float64 // for DCAOnSteroids
	AssetWeights map[string]float64
	LastClose    map[string]float64
	LastHigh     map[string]float64
	AssetStake   map[string]float64
	Volume       map[string]float64
	ATHTest      map[string]float64
	Slugs        map[string]string
}

func NewStrategyData(config *Config) (*StrategyData, error) {
	slugs, err := getSlugs(config.AssetWeights)
	if err != nil {
		return nil, err
	}

	return &StrategyData{
		//MinimumBalance:    config.MinimumBalance,
		//ExpectedPriceDrop: config.ExpectedPriceDrop,
		AssetWeights: config.AssetWeights,
		LastClose:    make(map[string]float64),
		LastHigh:     make(map[string]float64),
		AssetStake:   make(map[string]float64),
		Volume:       make(map[string]float64),
		// Last ATH
		/*ATHTest: map[string]float64{
			"BTCUSDT":  68972.0,
			"ADAUSDT":  3.1016,
			"ETHUSDT":  4886.0,
			"SOLUSDT":  259.0,
			"BNBUSDT":  692.2,
			"XRPUSDT":  1.9706,
			"DOTUSDT":  55.0,
			"UNIUSDT":  44.357,
			"AVAXUSDT": 146.76,
			"LINKUSDT": 53.08,
			"TRXUSDT":  0.1803,
			"TONUSDT":  10.0,
			"HBARUSDT": 0.57512,
			"XLMUSDT":  0.797,
			"SUIUSDT":  10.0,
		},*/
		ATHTest: map[string]float64{
			"BTCUSDT":  0.0,
			"ADAUSDT":  0.0,
			"ETHUSDT":  0.0,
			"SOLUSDT":  0.0,
			"BNBUSDT":  0.0,
			"XRPUSDT":  0.0,
			"DOTUSDT":  0.0,
			"UNIUSDT":  0.0,
			"AVAXUSDT": 0.0,
			"LINKUSDT": 0.0,
			"TRXUSDT":  0.0,
			"TONUSDT":  0.0,
			"HBARUSDT": 0.0,
			"XLMUSDT":  0.0,
			"SUIUSDT":  0.0,
		},
		Slugs: slugs,
	}, nil
}

// Get the slug (coin ID) values for each of the assets in the map
func getSlugs(assetWeights map[string]float64) (map[string]string, error) {
	cacheMutex.Lock()
	defer cacheMutex.Unlock()

	// 检查缓存是否过期（1分钟）
	if time.Since(cacheTime) > time.Minute {
		resp, err := resty.New().R().
			Get("https://api.coingecko.com/api/v3/coins/list")
		if err != nil {
			return nil, err
		}
		var jsonResp []map[string]string
		if err := json.Unmarshal(resp.Body(), &jsonResp); err != nil {
			return nil, err
		}
		if len(jsonResp) == 0 {
			return nil, errors.New(noCoinsListErr)
		}

		// 更新缓存
		cacheData = jsonResp
		cacheTime = time.Now()
	}

	slugs := make(map[string]string)

	for pair := range assetWeights {
		symbol := strings.Split(pair, USDSymbol)[0]

		if slug, ok := findSlug(symbol); ok {
			slugs[pair] = slug
		} else {
			return nil, fmt.Errorf(coinNotFoundErr, symbol)
		}
	}

	return slugs, nil
}

func findSlug(symbol string) (string, bool) {
	for _, coin := range cacheData {
		if coin["symbol"] == strings.ToLower(symbol) {
			return coin["id"], true
		}
	}
	return "", false
}
