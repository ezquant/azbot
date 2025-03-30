# azbot

[//]: ![Azbot](https://user-images.githubusercontent.com/7620947/161434011-adc89d1a-dccb-45a7-8a07-2bb55e62d2d9.png)
[//]: [![tests](https://github.com/ezquant/azbot/actions/workflows/ci.yaml/badge.svg)](https://github.com/ezquant/azbot/actions/workflows/ci.yaml)
[//]: [![codecov](https://codecov.io/gh/ezquant/azbot/branch/main/graph/badge.svg)](https://codecov.io/gh/ezquant/azbot)
[//]: [![GoReference](https://pkg.go.dev/badge/github.com/ezquant/azbot.svg)](https://pkg.go.dev/github.com/ezquant/azbot)
[//]: [![Discord](https://img.shields.io/discord/960156400376483840?color=5865F2&label=discord)](https://discord.gg/TGCrUH972E)
[//]: [![Discord](https://img.shields.io/badge/donate-patreon-red)](https://www.patreon.com/azbot_github)
[//]: [![Docs](https://ezquant.github.io/azbot/)](https://ezquant.github.io/azbot/)

A fast cryptocurrency and stock trading bot framework implemented in Go. Azbot permits users to create and test custom strategies for spot and future markets.

| DISCLAIMER |
| ---------- |
| This software is for educational purposes only. Do not risk money which you are afraid to lose. USE THE SOFTWARE AT YOUR OWN RISK. THE AUTHORS AND ALL AFFILIATES ASSUME NO RESPONSIBILITY FOR YOUR TRADING RESULTS. |

## Install

### Build

```sh
go mod tidy
make clean
make
```

### CLI (optional)

- Pre-build binaries in [release page](https://github.com/ezquant/azbot/releases)
- Or with `go install github.com/ezquant/azbot/cmd/azbot@latest`

## Usage

Check [examples](examples) directory:

- Paper Wallet (Live Simulation)
- Backtesting (Simulation with historical data)
- Real Account (Binance)

## Download Historical Data

To download historical data you can download azbot CLI from:

```sh
# Use SOCKS5 proxy, if needed
export HTTPS_PROXY="socks5://127.0.0.1:1081"

# Download candles of BTCUSDT to CSV file (Last 30 days, timeframe 1D)
./dist/bin/azbot download --pair BTCUSDT --timeframe 1d --days 30 --output ./testdata/BTCUSDT-5m.csv
```

## Backtesting Example

Backtesting a custom strategy from [examples](examples) directory:

```sh
./dist/bin/azbot backtest
```

Output:

```text
time="2025-03-25 18:02" level=info msg="[SETUP] Using paper wallet"
time="2025-03-25 18:02" level=info msg="[SETUP] Initial Portfolio = 10000.000000 USDT"
 100% |███████████████████████████████████████████████████████████████████████████████████████████████████████████████████████████████████| (8626/8626, 226268 it/s)         
+---------+--------+-----+------+--------+--------+-----+----------+-----------+
|  PAIR   | TRADES | WIN | LOSS | % WIN  | PAYOFF | SQN |  PROFIT  |  VOLUME   |
+---------+--------+-----+------+--------+--------+-----+----------+-----------+
| BTCUSDT |     14 |   6 |    8 | 42.9 % |  5.929 | 1.5 | 13511.66 | 448030.05 |
| ETHUSDT |      9 |   6 |    3 | 66.7 % |  3.407 | 1.3 | 21748.41 | 407769.64 |
+---------+--------+-----+------+--------+--------+-----+----------+-----------+
|   TOTAL |     23 |  12 |   11 | 52.2 % |  4.942 | 1.4 | 35260.07 | 855799.68 |
+---------+--------+-----+------+--------+--------+-----+----------+-----------+

-- FINAL WALLET --
0.0000 BTC = 0.0000 USDT
0.0000 ETH = 0.0000 USDT
45260.0735 USDT

----- RETURNS -----
START PORTFOLIO     = 10000.00 USDT
FINAL PORTFOLIO     = 45260.07 USDT
GROSS PROFIT        =  35260.073493 USDT (352.60%)
MARKET CHANGE (B&H) =  407.09%

------ RISK -------
MAX DRAWDOWN = -11.76 %

------ VOLUME -----
BTCUSDT         = 448030.05 USDT
ETHUSDT         = 407769.64 USDT
TOTAL           = 855799.68 USDT
-------------------
Chart available at http://localhost:8080
```

### Plot result

<img width="100%"  src="https://user-images.githubusercontent.com/7620947/139601478-7b1d826c-f0f3-4766-951e-b11b1e1c9aa5.png" />

## Features

|                    	| Binance Spot 	| Binance Futures 	 |
|--------------------	|--------------	|------------------- |
| Order Market       	|       :ok:    | :ok:               |
| Order Market Quote 	|       :ok:    | 	                 |
| Order Limit        	|       :ok:    | :ok:               |
| Order Stop         	|       :ok:    | :ok:               |
| Order OCO          	|       :ok:    | 	                 |
| Backtesting        	|       :ok:    | :ok:         	     |

## Roadmap

- [x] Backtesting
  - [x] Order Limit, Market, Stop Limit, OCO
  - [x] Load Feed from CSV
  - [ ] Load Feed from TDX local files (new)
  - [ ] Load Feed from Redis DB (new)

- [x] Paperwallet
  - [x] Paper Wallet (Live Trading with fake wallet)
  - [x] Load Feed from Binance
  - [ ] Load Feed from TDX remote API (new)
  - [ ] Load Feed from CTP broker (new)

- [x] Bot Utilities
  - [x] CLI to download historical data
  - [x] Plot (Candles + Sell / Buy orders, Indicators)
  - [x] Telegram Controller (Status, Buy, Sell, and Notification)
  - [x] Heikin Ashi candle type support
  - [x] Trailing stop tool
  - [x] In app order scheduler

- [x] Strategies
  - [x] Emacross
  - [x] Ocosell
  - [x] Trailingstop
  - [x] Turtle
  - [ ] Mean Reversion (new)
  - [ ] DCAOnSteroids (new)
  - [ ] Diamondhands (new)
  - [ ] RL-PPO (new)

- [ ] Others
  - [ ] Include Web UI Controller
  - [ ] Include more chart indicators
  - [ ] Docs
  - [x] config file for strategy parameters (new)
  - [ ] Strategy parameter optimizer (new)
  - [ ] Docker support (new)

## New exchange

Currently, we only support [Binance](https://www.binance.com/en?ref=35723227) exchange. If you want to include support for other exchanges, you need to implement a new `struct` that implements the interface `Exchange`. You can check some examples in [exchange](./pkg/exchange) directory.

## Support the project

|  | Address  |
| --- | --- |
|**BTC** | `3EKTNjKNCmBqUZUZMFCuzpZkx7cna4GQ4S`|
|**ETH** | `0x0ba94169c2315635f2d66de6a53e69879b99be03` |

[comment]: <> (一段注释)
[comment]: # (一段注释)
[//]: // (一段注释)
[//]: 一段注释
[^_^]: 开心注释
[>_<]: 抓狂注释

## Thanks

The project was greatly inspired by: https://github.com/rodrigo-brito/ninjabot
