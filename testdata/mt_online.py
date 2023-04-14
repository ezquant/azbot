from mootdx.quotes import Quotes

# 标准市场
client = Quotes.factory(market='std', multithread=True, heartbeat=True, bestip=True, timeout=15)

# k 线数据
data = client.bars(symbol='600000', frequency=9, offset=1000)
print("--> data:", data)
#TODO: save to file

# 指数
#client.index(symbol='000001', frequency=9)

# 分钟
client.minute(symbol='000001')

