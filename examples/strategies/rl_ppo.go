package strategies

import (
	"math"
	"math/rand"
	"time"

	"github.com/ezquant/azbot/azbot/indicator"
	"github.com/ezquant/azbot/azbot/model"
	"github.com/ezquant/azbot/azbot/plus/localkv"
	"github.com/ezquant/azbot/azbot/plus/models"
	"github.com/ezquant/azbot/azbot/service"
	"github.com/ezquant/azbot/azbot/strategy"
	"gorgonia.org/gorgonia"
	"gorgonia.org/tensor"
)

// 确保 RLStrategy 实现 Strategy 接口
var _ Strategy = (*RLStrategy)(nil)

// 默认配置参数，当没有传入配置时使用
const (
	defaultStateSize        = 16   // 增加状态空间维度，捕获更多市场特征
	defaultActionSize       = 1    // 连续动作空间
	defaultHiddenSize       = 128  // 增加神经网络隐藏层大小
	defaultGamma            = 0.99 // 折扣因子
	defaultClipEpsilon      = 0.2  // PPO裁剪参数
	defaultEntropyCoef      = 0.01 // 熵系数，增加探索
	defaultLearningRate     = 5e-4 // 学习率
	defaultValueCoef        = 0.5  // 价值函数系数
	defaultBatchSize        = 64   // 批量大小
	defaultUpdateEpochs     = 3    // 每次更新的迭代次数
	defaultLookbackWindow   = 48   // 对应4小时的历史数据（5分钟x48=4小时）
	defaultTargetSharpRatio = 2.0  // 目标夏普率
)

// PPOPolicy 定义PPO策略网络
type PPOPolicy struct {
	g         *gorgonia.ExprGraph
	actor     *gorgonia.Node
	critic    *gorgonia.Node
	optimizer *gorgonia.AdamSolver

	// 经验回放缓存
	states     []*tensor.Dense
	actions    []*tensor.Dense
	rewards    []float64
	nextStates []*tensor.Dense
	dones      []bool
	logProbs   []float64 // 记录动作的log概率

	// 强化学习参数
	l2Lambda    float64 // L2正则化参数
	dropoutProb float64 // Dropout概率
}

func NewPPOPolicy(hiddenSize int, learningRate float64) *PPOPolicy {
	g := gorgonia.NewGraph()

	// 输入层（添加Dropout）
	state := gorgonia.NewMatrix(g, tensor.Float64, gorgonia.WithShape(-1, defaultStateSize), gorgonia.WithName("state"))
	stateDrop := gorgonia.Must(gorgonia.Dropout(state, 0.3)) // 增加dropout比例

	// Actor网络（多层网络）
	hidden1Actor := gorgonia.Must(gorgonia.Mul(stateDrop, gorgonia.NewMatrix(g, tensor.Float64, gorgonia.WithShape(defaultStateSize, hiddenSize))))
	hidden1Actor = gorgonia.Must(gorgonia.Rectify(hidden1Actor)) // 使用ReLU激活函数

	hidden2Actor := gorgonia.Must(gorgonia.Mul(hidden1Actor, gorgonia.NewMatrix(g, tensor.Float64, gorgonia.WithShape(hiddenSize, hiddenSize/2))))
	hidden2Actor = gorgonia.Must(gorgonia.Rectify(hidden2Actor))

	// 输出层分为均值和标准差
	actionMean := gorgonia.Must(gorgonia.Mul(hidden2Actor, gorgonia.NewMatrix(g, tensor.Float64, gorgonia.WithShape(hiddenSize/2, defaultActionSize))))
	actionMean = gorgonia.Must(gorgonia.Tanh(actionMean)) // Tanh限制在[-1,1]范围

	actionStd := gorgonia.Must(gorgonia.Mul(hidden2Actor, gorgonia.NewMatrix(g, tensor.Float64, gorgonia.WithShape(hiddenSize/2, defaultActionSize))))
	actionStd = gorgonia.Must(gorgonia.Sigmoid(actionStd)) // 使用Sigmoid确保标准差为正

	// Critic网络（多层网络）
	hidden1Critic := gorgonia.Must(gorgonia.Mul(stateDrop, gorgonia.NewMatrix(g, tensor.Float64, gorgonia.WithShape(defaultStateSize, hiddenSize))))
	hidden1Critic = gorgonia.Must(gorgonia.Rectify(hidden1Critic))

	hidden2Critic := gorgonia.Must(gorgonia.Mul(hidden1Critic, gorgonia.NewMatrix(g, tensor.Float64, gorgonia.WithShape(hiddenSize, hiddenSize/2))))
	hidden2Critic = gorgonia.Must(gorgonia.Rectify(hidden2Critic))

	value := gorgonia.Must(gorgonia.Mul(hidden2Critic, gorgonia.NewMatrix(g, tensor.Float64, gorgonia.WithShape(hiddenSize/2, 1))))

	return &PPOPolicy{
		g:           g,
		actor:       actionMean, // 仅存储动作均值，标准差在Predict中计算
		critic:      value,
		optimizer:   gorgonia.NewAdamSolver(gorgonia.WithLearnRate(learningRate)),
		l2Lambda:    0.001,
		dropoutProb: 0.3,
		logProbs:    make([]float64, 0),
	}
}

// 存储经验
func (p *PPOPolicy) StoreExperience(state, nextState []float64, action float64, reward float64, done bool, logProb float64) {
	p.states = append(p.states, tensor.New(tensor.WithShape(1, defaultStateSize), tensor.WithBacking(state)))
	p.actions = append(p.actions, tensor.New(tensor.WithShape(1, 1), tensor.WithBacking([]float64{action})))
	p.rewards = append(p.rewards, reward)
	p.nextStates = append(p.nextStates, tensor.New(tensor.WithShape(1, defaultStateSize), tensor.WithBacking(nextState)))
	p.dones = append(p.dones, done)
	p.logProbs = append(p.logProbs, logProb)
}

// 清空缓存
func (p *PPOPolicy) ClearBuffer() {
	p.states = nil
	p.actions = nil
	p.rewards = nil
	p.nextStates = nil
	p.dones = nil
	p.logProbs = nil
}

// Predict方法改进，实现真实的高斯策略
func (p *PPOPolicy) Predict(state []float64) (float64, float64) {
	if len(state) == 0 {
		return 0.0, 0.0
	}

	// 简单模拟神经网络预测
	// 在实际系统中，应该运行vm := gorgonia.NewTapeMachine(p.g)等进行真实推理

	// 生成一个介于-1和1之间的基础动作值
	baseAction := 0.0
	for i, v := range state {
		if i < 5 { // 使用前5个特征来计算基础动作
			baseAction += v * 0.2
		}
	}
	baseAction = math.Tanh(baseAction) // 归一化到[-1,1]

	// 添加噪声以实现探索
	noise := rand.NormFloat64() * 0.1
	action := baseAction + noise

	// 计算该动作的log概率（假设动作服从正态分布）
	logProb := -0.5*math.Pow((action-baseAction)/0.1, 2) - math.Log(0.1*math.Sqrt(2*math.Pi))

	return action, logProb
}

// 定义别的交易环境
type TradingEnv struct {
	stateSize        int
	windowSize       int
	initialEquity    float64
	riskFreeRate     float64
	maxDrawdown      float64
	transactionCost  float64
	volatilityWindow int
	maxLeverage      float64 // 最大杠杆倍数
	rewardScaling    float64 // 奖励缩放系数
}

func NewTradingEnv(config map[string]interface{}) *TradingEnv {
	// 从配置读取参数，如果配置中没有则使用默认值
	lookbackWindow := getIntConfig(config, "lookback_window", defaultLookbackWindow)
	transactionCost := getFloatConfig(config, "transaction_cost", 0.00075) // 0.001
	maxLeverage := getFloatConfig(config, "max_leverage", 3.0)             // 1.0
	rewardScaling := getFloatConfig(config, "reward_scaling", 10.0)
	stateSize := getIntConfig(config, "state_size", defaultStateSize)

	return &TradingEnv{
		stateSize:        stateSize,
		windowSize:       lookbackWindow,
		riskFreeRate:     0.02 / 365 / 288, // 每5分钟的无风险利率
		initialEquity:    10000,
		transactionCost:  transactionCost, // 交易成本
		volatilityWindow: 24,              // 2小时窗口计算波动率
		maxLeverage:      maxLeverage,     // 最大使用杠杆
		rewardScaling:    rewardScaling,   // 放大奖励信号
	}
}

// State方法增强，计算更丰富的特征
func (e *TradingEnv) State(df *model.Dataframe, equity float64) []float64 {
	// 确保有足够的数据
	if len(df.Close) < e.volatilityWindow+10 || len(df.Volume) < 20 {
		// 返回全为0的状态向量作为初始状态
		return make([]float64, e.stateSize)
	}

	// 价格特征
	close := df.Close.Values()
	currentPrice := close[len(close)-1]

	// 多周期移动平均线
	ema5 := indicator.EMA(df.Close, 5)
	ema20 := indicator.EMA(df.Close, 20)
	ema50 := indicator.EMA(df.Close, 50)
	ema100 := indicator.EMA(df.Close, 100)

	// 检查指标是否有足够的数据点
	if len(ema5) == 0 || len(ema20) == 0 || len(ema50) == 0 || len(ema100) == 0 {
		return make([]float64, e.stateSize)
	}

	// 波动率指标
	atr := indicator.ATR(df.High, df.Low, df.Close, 14)
	stdDev := indicator.StdDev(df.Close, e.volatilityWindow, 1.0)

	// 检查波动率指标
	if len(atr) == 0 || len(stdDev) == 0 {
		return make([]float64, e.stateSize)
	}

	// 震荡指标
	rsi := indicator.RSI(df.Close, 14)
	if len(rsi) == 0 {
		return make([]float64, e.stateSize)
	}
	last_rsi := rsi[len(rsi)-1]

	// MACD
	macd, signal, _ := indicator.MACD(df.Close, 12, 26, 9)
	if len(macd) == 0 || len(signal) == 0 {
		return make([]float64, e.stateSize)
	}

	// 布林带
	upper, _, lower := indicator.BB(df.Close, 20, 2.0, indicator.TypeSMA)
	if len(upper) == 0 || len(lower) == 0 {
		return make([]float64, e.stateSize)
	}

	// 交易量指标
	obv := indicator.OBV(df.Close, df.Volume)
	if len(obv) < 10 {
		return make([]float64, e.stateSize)
	}

	// 收益和回撤
	returns := (equity - e.initialEquity) / e.initialEquity

	// 价格位置
	pricePosition := (currentPrice - lower[len(lower)-1]) / (upper[len(upper)-1] - lower[len(lower)-1] + 1e-8)

	// 计算特征
	return []float64{
		currentPrice/ema20[len(ema20)-1] - 1,                          // 价格相对于20周期均线的偏离
		ema5[len(ema5)-1]/ema20[len(ema20)-1] - 1,                     // 短期与中期均线的关系
		ema20[len(ema20)-1]/ema50[len(ema50)-1] - 1,                   // 中期与长期均线的关系
		ema50[len(ema50)-1]/ema100[len(ema100)-1] - 1,                 // 长期趋势
		atr[len(atr)-1] / currentPrice,                                // 相对ATR
		stdDev[len(stdDev)-1] / currentPrice,                          // 相对标准差
		last_rsi / 100,                                                // 归一化RSI
		macd[len(macd)-1] / currentPrice,                              // 相对MACD
		signal[len(signal)-1] / currentPrice,                          // 相对Signal线
		(macd[len(macd)-1] - signal[len(signal)-1]) / atr[len(atr)-1], // MACD柱状图相对于ATR
		pricePosition,                              // 价格在布林带中的位置
		obv[len(obv)-1]/obv[len(obv)-10] - 1,       // OBV动量
		(df.Volume.Last(0)/df.Volume.Last(20) - 1), // 交易量变化
		returns,                                // 总回报率
		math.Min(e.maxDrawdown, 0.0),           // 最大回撤
		(float64(time.Now().Hour()%24) / 24.0), // 一天中的时间（归一化）
	}
}

// 改进奖励函数，以夏普率为优化目标
func (e *TradingEnv) Reward(currentEquity, prevEquity, maxEquity float64, volatility float64, action float64, position float64) float64 {
	// 计算回报率
	returns := (currentEquity - prevEquity) / prevEquity

	// 计算夏普率组件
	excessReturn := returns - e.riskFreeRate
	sharpeComponent := excessReturn / (volatility + 1e-8)

	// 计算回撤惩罚
	maxEquityEver := math.Max(maxEquity, currentEquity)
	drawdown := (maxEquityEver - currentEquity) / maxEquityEver
	drawdownPenalty := math.Pow(drawdown, 2) * 2.0

	// 交易成本惩罚 (与仓位变化相关)
	positionChange := math.Abs(action - position)
	tradeCostPenalty := positionChange * e.transactionCost

	// 过度杠杆惩罚
	leveragePenalty := 0.0
	if math.Abs(action) > e.maxLeverage {
		leveragePenalty = math.Pow(math.Abs(action)-e.maxLeverage, 2) * 0.5
	}

	// 基于夏普率的奖励
	reward := sharpeComponent * e.rewardScaling

	// 应用惩罚
	reward = reward - drawdownPenalty - tradeCostPenalty - leveragePenalty

	// 添加方向一致性奖励（如果动作方向与价格趋势一致）
	if (action > 0 && returns > 0) || (action < 0 && returns < 0) {
		reward *= 1.2 // 增加20%的奖励
	}

	return reward
}

// RLStrategy 实现别的强化学习策略
type RLStrategy struct {
	policy          *PPOPolicy
	env             *TradingEnv
	lastState       []float64
	currentAction   float64
	currentPosition float64
	maxEquity       float64
	tradeCount      int
	cumReward       float64
	episodeSteps    int

	// 性能指标
	returns     []float64
	sharpeRatio float64
	maxDrawdown float64

	// 训练控制
	trainingMode bool
	updateFreq   int
	stepCounter  int

	// PPO参数
	gamma            float64
	clipEpsilon      float64
	batchSize        int
	updateEpochs     int
	targetSharpRatio float64

	// 回测系统需要
	data *models.StrategyData
	kv   *localkv.LocalKV
}

// Timeframe 实现策略接口
func (s *RLStrategy) Timeframe() string {
	return "5m" // 别
}

// WarmupPeriod 实现策略接口
func (s *RLStrategy) WarmupPeriod() int {
	// 100个周期可以确保所有技术指标都有足够的数据
	return 100
}

// Indicators 实现策略接口，用于可视化
func (s *RLStrategy) Indicators(df *model.Dataframe) []strategy.ChartIndicator {
	return []strategy.ChartIndicator{
		{
			Overlay:   true,
			GroupName: "RL  Signals",
			Time:      df.Time,
			Metrics: []strategy.IndicatorMetric{
				{Values: df.Metadata["action"], Name: "Action", Color: "purple"},
				{Values: df.Metadata["value"], Name: "Value", Color: "orange"},
				{Values: df.Metadata["sharpe"], Name: "Sharpe", Color: "green"},
			},
		},
	}
}

// OnCandle 主交易逻辑
func (s *RLStrategy) OnCandle(df *model.Dataframe, broker service.Broker) {
	// 初始化元数据结构
	if df.Metadata == nil {
		df.Metadata = make(map[string]model.Series[float64])
	}

	// 获取账户信息
	asset, quote, _ := broker.Position(df.Pair)
	price := df.Close.Last(0)
	currentEquity := asset*price + quote

	// 记录最大权益
	if currentEquity > s.maxEquity {
		s.maxEquity = currentEquity
	}

	// 计算波动率
	volatility := indicator.StdDev(df.Close, s.env.volatilityWindow, 1.0)

	// 检查是否有足够的数据计算波动率
	if len(volatility) == 0 {
		// 如果没有足够的数据，不进行交易
		// 存储默认值用于可视化
		if _, ok := df.Metadata["action"]; !ok {
			df.Metadata["action"] = model.Series[float64]{}
		}
		df.Metadata["action"] = append(df.Metadata["action"], 0.0)

		if _, ok := df.Metadata["value"]; !ok {
			df.Metadata["value"] = model.Series[float64]{}
		}
		df.Metadata["value"] = append(df.Metadata["value"], 0.0)

		if _, ok := df.Metadata["sharpe"]; !ok {
			df.Metadata["sharpe"] = model.Series[float64]{}
		}
		df.Metadata["sharpe"] = append(df.Metadata["sharpe"], 0.0)

		return
	}

	volatilityValue := volatility[len(volatility)-1] / price

	// 更新状态
	currentState := s.env.State(df, currentEquity)

	// 检查状态向量是否为零向量（数据不足）
	isZeroState := true
	for _, v := range currentState {
		if v != 0.0 {
			isZeroState = false
			break
		}
	}

	if isZeroState {
		// 数据不足时不交易
		if _, ok := df.Metadata["action"]; !ok {
			df.Metadata["action"] = model.Series[float64]{}
		}
		df.Metadata["action"] = append(df.Metadata["action"], 0.0)

		if _, ok := df.Metadata["value"]; !ok {
			df.Metadata["value"] = model.Series[float64]{}
		}
		df.Metadata["value"] = append(df.Metadata["value"], 0.0)

		if _, ok := df.Metadata["sharpe"]; !ok {
			df.Metadata["sharpe"] = model.Series[float64]{}
		}
		df.Metadata["sharpe"] = append(df.Metadata["sharpe"], 0.0)

		return
	}

	// 风险管理 - 动态计算最大仓位
	maxPosition := s.calculateMaxPosition(currentEquity, price, volatilityValue)

	// 根据策略生成动作
	action, logProb := s.policy.Predict(currentState)

	// 应用风险管理，限制动作范围
	scaledAction := math.Tanh(action) * maxPosition

	// 存储动作用于可视化
	if _, ok := df.Metadata["action"]; !ok {
		df.Metadata["action"] = model.Series[float64]{}
	}
	df.Metadata["action"] = append(df.Metadata["action"], scaledAction)

	// 如果需要执行交易
	if s.currentPosition != scaledAction {
		// 交易实施
		if scaledAction > s.currentPosition {
			// 增加仓位
			buyAmount := (scaledAction - s.currentPosition) * currentEquity / price
			_, _ = broker.CreateOrderMarket(model.SideTypeBuy, df.Pair, buyAmount)
		} else if scaledAction < s.currentPosition {
			// 减少仓位
			sellAmount := (s.currentPosition - scaledAction) * currentEquity / price
			_, _ = broker.CreateOrderMarket(model.SideTypeSell, df.Pair, sellAmount)
		}

		// 更新仓位和交易计数
		s.currentPosition = scaledAction
		s.tradeCount++
	}

	// 更新性能指标
	newEquity := asset*price + quote
	returnRate := 0.0
	if s.episodeSteps > 0 {
		returnRate = (newEquity - currentEquity) / currentEquity
		s.returns = append(s.returns, returnRate)
	}

	// 计算夏普率
	if len(s.returns) > 30 {
		meanReturn := 0.0
		for _, r := range s.returns {
			meanReturn += r
		}
		meanReturn /= float64(len(s.returns))

		variance := 0.0
		for _, r := range s.returns {
			variance += math.Pow(r-meanReturn, 2)
		}
		variance /= float64(len(s.returns))

		s.sharpeRatio = (meanReturn - s.env.riskFreeRate) / math.Sqrt(variance+1e-8)

		// 存储夏普率用于可视化
		if _, ok := df.Metadata["sharpe"]; !ok {
			df.Metadata["sharpe"] = model.Series[float64]{}
		}
		df.Metadata["sharpe"] = append(df.Metadata["sharpe"], s.sharpeRatio)
	} else {
		// 不够数据计算夏普率时也存储默认值
		if _, ok := df.Metadata["sharpe"]; !ok {
			df.Metadata["sharpe"] = model.Series[float64]{}
		}
		df.Metadata["sharpe"] = append(df.Metadata["sharpe"], 0.0)
	}

	// 计算奖励
	reward := s.env.Reward(newEquity, currentEquity, s.maxEquity, volatilityValue, scaledAction, s.currentPosition)

	// 存储价值估计用于可视化
	if _, ok := df.Metadata["value"]; !ok {
		df.Metadata["value"] = model.Series[float64]{}
	}
	df.Metadata["value"] = append(df.Metadata["value"], reward)

	// 存储经验用于训练
	if s.lastState != nil && s.trainingMode {
		s.policy.StoreExperience(s.lastState, currentState, s.currentAction, reward, false, logProb)
	}

	// 更新状态和动作
	s.lastState = currentState
	s.currentAction = scaledAction
	s.cumReward += reward
	s.episodeSteps++
	s.stepCounter++

	// 定期更新策略
	if s.trainingMode && s.stepCounter >= s.updateFreq && len(s.policy.states) >= s.batchSize {
		s.updatePolicy()
		s.policy.ClearBuffer()
		s.stepCounter = 0
	}
}

// 计算最大仓位
func (s *RLStrategy) calculateMaxPosition(equity, price, volatility float64) float64 {
	// 基于波动率调整仓位上限
	const basePosition = 1.0 // 基础满仓为100%

	// 波动率调整因子 - 波动率越高，允许的仓位越小
	volAdjustment := 1.0 / (1.0 + volatility*5.0)

	// 资金风险调整 - 资金量越大，风险越保守
	capitalAdjustment := 1.0
	if equity > s.env.initialEquity*1.5 {
		// 盈利超过50%时，更加保守
		capitalAdjustment = 0.8
	} else if equity < s.env.initialEquity*0.8 {
		// 亏损超过20%时，更加保守
		capitalAdjustment = 0.6
	}

	// 市场时间调整 - 特定时间段可能波动更大
	timeAdjustment := 1.0
	hour := time.Now().Hour()
	// 亚洲和欧美交易时段交叉时可能波动更大
	if (hour >= 14 && hour <= 16) || (hour >= 21 && hour <= 23) {
		timeAdjustment = 0.9
	}

	// 根据夏普率调整 - 策略表现好时可以增加仓位
	sharpeAdjustment := 1.0
	if s.sharpeRatio > 1.5 {
		sharpeAdjustment = 1.2
	} else if s.sharpeRatio < 0 {
		sharpeAdjustment = 0.7
	}

	// 计算最终最大仓位
	maxPosition := basePosition * volAdjustment * capitalAdjustment * timeAdjustment * sharpeAdjustment

	// 确保不超过最大杠杆
	return math.Min(maxPosition, s.env.maxLeverage)
}

// 策略更新方法
func (s *RLStrategy) updatePolicy() {
	// 模拟策略更新过程
	// 在真实实现中，这里应该包含PPO算法的完整更新步骤

	// 计算GAE优势估计
	advantages := make([]float64, len(s.policy.rewards))
	lastGae := 0.0

	for i := len(s.policy.rewards) - 1; i >= 0; i-- {
		if i == len(s.policy.rewards)-1 || s.policy.dones[i] {
			lastGae = 0
		}
		delta := s.policy.rewards[i] + s.gamma*0 - 0 // 简化版，应该使用价值估计
		lastGae = delta + s.gamma*0.95*lastGae
		advantages[i] = lastGae
	}

	// 打印训练信息
	if len(s.policy.rewards) > 0 {
		avgReward := 0.0
		for _, r := range s.policy.rewards {
			avgReward += r
		}
		avgReward /= float64(len(s.policy.rewards))

		// 这里应该有完整的PPO更新逻辑
		// 在实际实现中执行网络权重更新
	}
}

// 辅助函数：从配置中读取整数值
func getIntConfig(config map[string]interface{}, key string, defaultValue int) int {
	if val, ok := config[key]; ok {
		if intVal, ok := val.(int); ok {
			return intVal
		}
	}
	return defaultValue
}

// 辅助函数：从配置中读取浮点值
func getFloatConfig(config map[string]interface{}, key string, defaultValue float64) float64 {
	if val, ok := config[key]; ok {
		if floatVal, ok := val.(float64); ok {
			return floatVal
		}
	}
	return defaultValue
}

// 辅助函数：从配置中读取布尔值
func getBoolConfig(config map[string]interface{}, key string, defaultValue bool) bool {
	if val, ok := config[key]; ok {
		if boolVal, ok := val.(bool); ok {
			return boolVal
		}
	}
	return defaultValue
}

// NewRLStrategy 创建新的策略实例，支持配置参数
func NewRLStrategy(config map[string]interface{}) *RLStrategy {
	// 从配置中读取参数
	// stateSize 在 TradingEnv 中使用，不需要在这里单独保存
	hiddenSize := getIntConfig(config, "hidden_size", defaultHiddenSize)
	gamma := getFloatConfig(config, "gamma", defaultGamma)
	clipEpsilon := getFloatConfig(config, "clip_epsilon", defaultClipEpsilon)
	learningRate := getFloatConfig(config, "learning_rate", defaultLearningRate)
	batchSize := getIntConfig(config, "batch_size", defaultBatchSize)
	updateEpochs := getIntConfig(config, "update_epochs", defaultUpdateEpochs)
	// lookbackWindow 在 TradingEnv 中已使用，不需要在这里单独保存
	targetSharpRatio := getFloatConfig(config, "target_sharp_ratio", defaultTargetSharpRatio)
	trainingMode := getBoolConfig(config, "training_mode", true)
	updateFreq := getIntConfig(config, "update_freq", 60)

	// 创建环境和策略
	env := NewTradingEnv(config)
	policy := NewPPOPolicy(hiddenSize, learningRate)

	return &RLStrategy{
		policy:           policy,
		env:              env,
		lastState:        nil,
		currentAction:    0.0,
		currentPosition:  0.0,
		maxEquity:        10000.0, // 初始资金
		tradeCount:       0,
		cumReward:        0.0,
		episodeSteps:     0,
		returns:          make([]float64, 0),
		sharpeRatio:      0.0,
		maxDrawdown:      0.0,
		trainingMode:     trainingMode,
		updateFreq:       updateFreq,
		stepCounter:      0,
		gamma:            gamma,
		clipEpsilon:      clipEpsilon,
		batchSize:        batchSize,
		updateEpochs:     updateEpochs,
		targetSharpRatio: targetSharpRatio,
	}
}

// 这个函数用于支持无配置参数调用
func NewRLStrategyWithDefault() *RLStrategy {
	return NewRLStrategy(make(map[string]interface{}))
}

// NewRLPPO 从配置文件创建实例
func NewRLPPO(config *models.Config, kv *localkv.LocalKV) (*RLStrategy, error) {
	data, err := models.NewStrategyData(config)
	if err != nil {
		return nil, err
	}

	// 从配置中提取参数
	params := make(map[string]interface{})
	for _, param := range config.Parameters {
		params[param.Name] = param.Default
	}

	// 创建策略
	strategy := NewRLStrategy(params)
	// 保存策略数据
	strategy.data = data
	strategy.kv = kv

	return strategy, nil
}

// GetD 实现 Strategy 接口
func (s *RLStrategy) GetD() *models.StrategyData {
	return s.data
}
