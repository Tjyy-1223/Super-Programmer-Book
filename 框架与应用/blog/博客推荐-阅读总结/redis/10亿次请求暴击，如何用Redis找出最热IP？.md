## 10亿次请求暴击，如何用Redis找出最热IP？

#### 问题与方案概述

问题：亿级数据下的"最热IP统计"难题

+ 当前有10亿次请求正在冲击服务器，导致 CPU 服务器飙升到 100%
+ 如何查找到哪个 IP 正在进行攻击

真实案例：某社交APP用Hash方案统计UV，服务器每天凌晨准时崩溃，改用HLL后成本直降99%

方案对比：

| 方案                   | 内存占用 | 查询耗时 | 精确度 | 致命伤                 |
| ---------------------- | -------- | -------- | ------ | ---------------------- |
| Hash                   | 38.2 GB  | 1200ms   | 100%   | OOM                    |
| ZSet                   | 41.7 GB  | 1500ms   | 100%   | OOM                    |
| HLL + Bitmap + Zset    | 3 GB     | 2ms      | 99.2%  | 需要组合使用，比较复杂 |
| RedisBloom TopK + 分片 | 3.8 GB   | 50ms     | 100%   | 需要 Redis 5.0 版本    |

**实战总结：**

1. 10万级以下：直接用ZSet
2. 百万~十亿级： 使用 RedisBloom TopK + 分片集群
3. 百亿级以上：加上限流策略，将流量控制在百万-十亿级



#### 方案 1： 传统 Hash

Redis 的 `Hash` 结构其实就是一个 `key -> field -> value` 的映射，非常适合用来做 IP 计数器。我们将所有请求的 IP 统计进一个 Redis Hash：

- Redis key：`"ip_counter"`
- Hash field：IP 地址（如 `"192.168.1.1"`）
- Hash value：计数值（如 `1234`）

示例操作：

1. 有 10 亿次请求，每个请求通过 IP 执行一次 `HINCRBY`。
2. Redis 自动维护每个 IP 的计数。
3. 最后可以通过遍历 Redis Hash 获取最热 IP。

```
# 发送每个请求：
HINCRBY ip_counter 192.168.1.1 1
```

查询最热 IP 的脚本（伪代码）：

```lua
local max_ip = nil
local max_count = 0
for ip, count in HGETALL("ip_counter") do
    if count > max_count then
        max_ip = ip
        max_count = count
    end
end
return max_ip, max_count
```

> 或者从客户端（如 Python、Go）使用 `HGETALL` 后在本地找最大值。

**内存使用估算：**

每个 IP 大概是 15 字节（IPv4），再加上 Redis 字符串封装开销，每个 Hash field 约占 40-60 字节。如果是 1 亿个唯一 IP：

- 平均每个 Hash 约占 80B（key+field+value 总和）
- 80 * 100,000,000 = **~8 GB** 左右

再加上 Redis 的内部结构、网络 buffer、持久化等等，Redis 很容易达到内存瓶颈。

**致命伤：**

1. 内存占用高：Hash 结构按 IP 精确存储，IP 越多越爆
2. 查询不高效：Redis 的 `HGETALL` 会一次性把所有 IP 都拉出来，非常慢（甚至阻塞）
3. 单线程性能瓶颈：Redis 是单线程的，QPS 被拉满后响应能力变差
4. 无法近似估计：没有近似算法机制，必须全量处理所有 IP

**优点：**

1. 精度高：和原始 HashMap 一样，统计是 100% 精确的
2. 代码简单：直接使用 `HINCRBY` 即可
3. 可持久化：数据可落地，可在断电后恢复



#### 方案 2：ZSet 

使用 Redis 的 Sorted Set（ZSet） 来统计“亿级请求下的最热 IP”是一个比 Hash 更适合做 TopN 排行的方案，特别是当你只关心出现次数最多的 IP（Top 1 / Top K）而非精确记录每个 IP。实现思路：

- 使用 ZSet（有序集合）记录 IP 出现次数
- Redis Key：`"ip_zset"`
- ZSet 的 **member 是 IP**，**score 是次数**

每次请求时：

```
ZINCRBY ip_zset 1 192.168.1.1
```

这个命令的含义是：给 IP `192.168.1.1` 的计数 `+1`，如果 IP 不存在会自动插入，score 为 1

查询最热 IP（Top 1）：

```
ZREVRANGE ip_zset 0 0 WITHSCORES
```

含义：从 ZSet 中按 score 降序返回前 1 个 IP（即请求次数最多的）

查询前 N 热 IP（Top N）：

```
ZREVRANGE ip_zset 0 N-1 WITHSCORES
```

**内存占用估算：**

- 每个 IP 的 member 是字符串，假设平均 15 字节
- 每个 ZSet 元素由跳表和哈希结构维护，开销比 Hash 大，但更适合排序查询
- 估算比 Hash 高约 **20-30% 内存**

如：1 亿个唯一 IP，约占用 10~12 GB 内存

**操作复杂度：**

| 操作        | 复杂度      | 性能表现       |
| ----------- | ----------- | -------------- |
| `ZINCRBY`   | O(logN)     | 较快，跳表结构 |
| `ZREVRANGE` | O(logN + M) | 快速获取 TopN  |

**优点：**

1. 实时 TopN：内建排序结构，支持快速获取“最热 IP”
2. 近似热度监控：不需要关心所有 IP，只跟踪高频 IP
3. 内存可控：对比 Hash 精度，ZSet 用于 TopN 更经济

**缺点：**

1. 插入速度略低:`ZINCRBY` 是 logN 复杂度（比 Hash 慢）
2. 内存仍然较大：IP 太多时仍可能爆内存（不过比 Hash 好）
3. 精度不是关键优势：如果你要全量精确计数，Hash 更好



#### 方案3：HLL + Bitmap + Zset

HLL（HyperLogLog） 是 Redis 中专门用于 “基数统计” 的数据结构 —— 也就是用来统计有多少个不重复元素（例如独立访问用户数、IP总数等）。

> `HyperLogLog (HLL)` 是一个非常神奇又高效的算法，它能在只用 12 KB 左右内存的情况下，估算上亿个不同元素的去重总数（即基数，Cardinality），精度误差仅约 0.81%。
>
> HyperLogLog 是一个“用概率 + 哈希”换时间和空间的方案，特别适合“超大规模数据去重计数”，如：活跃 IP、去重用户数、商品浏览数等。
>
> ```go
> PFADD ip_hll 192.168.0.1
> PFADD ip_hll 10.0.0.1
> PFADD ip_hll 192.168.0.1  # 不会重复计数
> PFCOUNT ip_hll            # 统计不重复 IP 数量（基数）
> ```

**HLL 不适合直接用来统计“最热 IP”**，但可以间接配合其他结构使用，做 **高频 IP 检测** 或攻击源识别的“预估+过滤”层。

> 为什么 HLL 不能统计最热 IP？
>
> 1. 只统计“有多少个唯一元素”：它只能告诉你“有多少个不同 IP”
> 2. 不记录每个元素出现次数：所以你无法得知哪个 IP 更热
> 3. 精度可接受 + 内存极省：只需 12 KB 就能统计上亿不同 IP 的数量

**那 HLL 可以起到的作用是什么？**

1. 估算活跃 IP 数：`PFADD ip_hll <ip>`
2. 快速预判是否“IP 爆炸”：判断当前活跃 IP 是否异常增长
3. 多节点合并去重 IP：`PFMERGE all_ips ip_hll1 ip_hll2`

**如果要做真正的“高频 IP 检测系统”，可以进行的设计如下：**

- HyperLogLog：估算活跃 IP 总量
- Bitmap：快速判断 IP 是否出现过
- ZSet：追踪频繁访问的 IP
- 滑动窗口 + TTL：做时间窗口隔离，防止冷数据干扰
- 告警规则：用于识别可能攻击的“热 IP”

架构图：

```sql
			 +----------------------+
       |      请求日志        |
       +----------+-----------+
                  |
                  v
      +---------------------------+
      |   IP -> 整数 offset 映射   |
      +---------------------------+
                  |
        +---------+--------+
        |                  |
        v                  v
+----------------+   +------------------+
| PFADD ip_hll   |   | SETBIT bitmap_k  |
| (基数统计)     |   | (判重)           |
+----------------+   +------------------+
                            |
                            v
               +------------------------+
               | 如果 GETBIT 返回 1      |
               | --> ZINCRBY ip_zset    |
               | (频率累加)              |
               +------------------------+
                            |
                            v
               +------------------------+
               | ZREVRANGE ip_zset      |
               | 获取 TopN 热 IP         |
               +------------------------+
```

1. IP 映射为整数（比如 IPv4）：

```go
func ipToOffset(ip string) uint32 {
    // 简化代码，真实情况需校验合法性
    parts := strings.Split(ip, ".")
    return uint32(atoi(parts[0])<<24 | atoi(parts[1])<<16 | atoi(parts[2])<<8 | atoi(parts[3]))
}
```

2. Redis 操作逻辑：

```go
# 1. 活跃 IP 判断（大概总量）：
redis.pfadd("ip_hll:20250423", ip)

# 2. 是否是第一次出现（去重）：
offset = ip_to_offset(ip)
first_seen = redis.setbit("ip_bitmap:20250423", offset, 1) == 1

# 3. 如果不是第一次，就统计频率：
if first_seen:
    redis.zincrby("ip_zset:20250423", 1, ip)

# 4. 定时查询“热 IP”排行：
top_ips = redis.zrevrange("ip_zset:20250423", 0, 9, withscores=True)
```

3. 滑动时间窗口处理（防冷数据干扰）：建议按“分钟”或“5分钟”为单位存储 Redis Key：

```go
ip_bitmap:20250423:1200
ip_hll:20250423:1200
ip_zset:20250423:1200
```

然后使用 `EXPIRE` 或 `SETEX` 设置 TTL，比如 `EXPIRE 3600`（1小时）；这样每分钟清除一次旧数据，确保热度统计聚焦在近实时 IP 活跃度。

4. **攻击检测规则（可配置）**

+ 总 IP 增速：每分钟新增 > 10000，代表可疑 IP 爆发增长
+ 某 IP 频次：单 IP 1 分钟内 > 500，代表热 IP 可能在攻击
+ 多个 IP 同段频次过高：某 IP 段累计 > 10000，代表某个 C 段爆发，可能是 DDoS 源段
+ 热 IP 多次跨分钟持续出现：同一 IP 连续 Top3 热，代表持续行为
+ 对应特定的行为进行分析，例如只访问特定API的IP以及突发的境外IP请求。

5. 整体优缺点分析

| 项目         | 优点                                       | 缺点                       |
| ------------ | ------------------------------------------ | -------------------------- |
| 性能         | 全部为 O(1) 或 O(logN)，可支撑亿级请求     | IP 转换、维护窗口略繁琐    |
| 精度         | 精确统计频率，ZSet 支持 TopN               | Bitmap 和 HLL 是估算或判重 |
| 可扩展性     | 可分 shard（如按网段、机房、时间分库）     | 需要 Redis 支持较好内存    |
| 组合策略灵活 | 可快速增加报警规则、封禁、地理分析等子系统 | 系统设计复杂度稍高         |



#### 方案4：RedisBloom TopK 方案 

**什么是 RedisBloom 模块？**

`RedisBloom` 是 Redis 官方的一个模块，专门提供概率性数据结构，适用于以下场景：

1. Bloom Filter：判断某个值“可能存在”（但有误判）
2. Cuckoo Filter：与 Bloom 类似但支持删除
3. Count-Min Sketch：估算频率（高并发热词、IP 频率）
4. Top-K：高效维护 TopN 热点（热门商品、攻击 IP 等）
5. T-Digest：高性能分位数估计（如 P99 响应时间）

**TopK 是 RedisBloom 模块中提供的“近似排序频率统计”工具，核心功能是：**

1. 快速识别流式数据中频率最高的前 K 个元素
2. 插入快、内存占用固定、统计近似准确

**TopK 背后的原理简述**

- 基于 **Count-Min Sketch + 小顶堆**
- 每个元素插入时会更新其频率估值
- 如果它足够“热”，会替换掉当前堆中最不热门的元素
- 保证我们始终有 **最热的 K 个元素** 被维护

**初始化一个 TopK 数据结构（只做一次）：**

```
TOPK.RESERVE ip_topk 1000 20000 7 0.925
```

| 参数    | 含义                     |
| ------- | ------------------------ |
| `1000`  | 保留 Top 1000 热 IP      |
| `20000` | 频率估算用的 sketch 宽度 |
| `7`     | 哈希冲突容忍度           |
| `0.925` | 精度容忍系数             |

每次统计一个 IP：每次 IP 请求都加 1，系统自动维护热度

```
TOPK.INCRBY ip_topk 192.168.1.1 1
```

查询当前 TopK 热门 IP：

```
TOPK.LIST ip_topk
```

查询某些 IP 的估算访问频率：

```
TOPK.COUNT ip_topk 192.168.1.1 10.0.0.2
```

**Gin + RedisBloom TopK 实现 IP 热度追踪服务**

```go
var (
    ctx = context.Background()
    rdb = redis.NewClient(&redis.Options{
        Addr: "localhost:6379",
    })
)

func main() {
    r := gin.Default()

    // 跟踪 IP 热度
    r.POST("/track_ip", func(c *gin.Context) {
        ip := c.ClientIP()
        if ip == "" {
            c.JSON(http.StatusBadRequest, gin.H{"error": "missing IP"})
            return
        }
        _, err := rdb.Do(ctx, "TOPK.INCRBY", "ip_topk", ip, 1).Result()
        if err != nil {
            c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
            return
        }
        c.JSON(http.StatusOK, gin.H{"status": "tracked", "ip": ip})
    })

    // 获取当前热 IP 列表
    r.GET("/top_ips", func(c *gin.Context) {
        result, err := rdb.Do(ctx, "TOPK.LIST", "ip_topk").StringSlice()
        if err != nil {
            c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
            return
        }
        c.JSON(http.StatusOK, gin.H{"top_ips": result})
    })

    // 查询某个 IP 热度
    r.GET("/ip_count", func(c *gin.Context) {
        ip := c.Query("ip")
        if ip == "" {
            c.JSON(http.StatusBadRequest, gin.H{"error": "missing IP"})
            return
        }
        count, err := rdb.Do(ctx, "TOPK.COUNT", "ip_topk", ip).IntSlice()
        if err != nil || len(count) == 0 {
            c.JSON(http.StatusInternalServerError, gin.H{"error": "lookup failed"})
            return
        }
        c.JSON(http.StatusOK, gin.H{"ip": ip, "count": count[0]})
    })

    r.Run(":8080")
}
```

使用示例：

- `POST http://localhost:8080/track_ip` 自动追踪来源 IP
- `GET  http://localhost:8080/top_ips` 获取 TopK 热 IP
- `GET  http://localhost:8080/ip_count?ip=1.2.3.4` 获取 IP 热度

当数据量过大时，上述方案如何结合redis分片集群。这是个非常关键的问题：**当 IP 数据量极大（上亿），一个 Redis 实例撑不住时，如何在 Redis 分片集群中高效使用 Bloom/TopK？**

问题的核心点在于，RedisBloom 的数据结构（如 Bloom、TopK）虽然轻量，但：

- 单个结构仍有大小上限
- 在 Redis 分片集群中，这些结构默认是不能自动分布式扩展的（即结构必须在某个 slot）

因此：需要人为做 数据分片+结构命名空间隔离

**方案：按 IP hash + 多结构分片**

步骤如下：

1. 拆分多个 TopK 结构（如 1024 个）

   ```go
   TOPK.RESERVE ip_topk_000 1000 20000 7 0.925
   ...
   TOPK.RESERVE ip_topk_1023 ...
   ```

   你可以通过寻找分片：

   ```go
   index := crc32.ChecksumIEEE([]byte(ip)) % 1024
   ```

   然后执行：

   ```go
   rdb.Do(ctx, "TOPK.INCRBY", fmt.Sprintf("ip_topk_%03d", index), ip, 1)
   ```

2. 查询 Top IP 时，并发聚合所有子结构
   + `TOPK.LIST` 每个结构返回前 K 个 IP
   + 聚合结果后再用本地排序得到全局前 K（例如用 `heap`）

代码片段：自动选择分片 TopK

```go
func getTopKKey(ip string) string {
    idx := crc32.ChecksumIEEE([]byte(ip)) % 1024
    return fmt.Sprintf("ip_topk_%03d", idx)
}

// 记录某 IP
func trackIP(ip string) {
    key := getTopKKey(ip)
    _, _ = rdb.Do(ctx, "TOPK.INCRBY", key, ip, 1).Result()
}
```

优化建议：

| 技术      | 用法                                          |
| --------- | --------------------------------------------- |
| Lua 脚本  | 让 `TOPK.INCRBY + 封禁检测` 在 Redis 内完成   |
| 延迟聚合  | 每 10 秒聚合所有 topk 的 `LIST`，入缓存在本地 |
| 熔断热 IP | 结合 bitmap 或 Bloom 做频率门控，减少攻击面   |

在 Redis 分片集群中使用 RedisBloom TopK，需要 **手动进行结构分片 + 聚合查询**，但这套方案：

1. 支持水平扩展到亿级 IP
2. 热 IP 精度仍然可靠（允许近似误差）
3. 查询性能与稳定性更高



#### **RedisBloom 为什么比 hash 和 zset 更节省内存？**

**首先看传统方案内存开销**

| 结构类型 | 每个 IP 占用内存（大概） | 特点                  |
| -------- | ------------------------ | --------------------- |
| Hash     | ~40–100 字节/IP          | 精准，空间大          |
| ZSet     | ~100–200 字节/IP         | 排序 + 分数，空间更大 |

10 亿 IP × 100 字节 = 100 GB！

ZSet 还要存字符串 IP + 分数（浮点），维护跳表和压缩表结构，尤其是写多读少场景下，性能和内存双压力。

**RedisBloom TopK 节省内存的核心逻辑**

1. 固定大小（常数空间）结构

   ```
   TOPK.RESERVE ip_topk 1000 20000 7 0.925
   ```

   - 保留最多 1000 个热 IP
   - 内部 bucket、计数、冲突区设计好
   - 不管你喂给它 1 万 IP 还是 10 亿 IP，它都只维护 **最有可能在前 1000 个的 IP**         

2. 使用 Count-Min Sketch 做估计统计，节省频率计数空间
   + 频率不是用 `map[ip]count` 存的
   + 而是用压缩的哈希矩阵（Count-Min Sketch），近似计数

| 场景      | Hash  | ZSet   | RedisBloom TopK     |
| --------- | ----- | ------ | ------------------- |
| 1 万 IP   | ~1MB  | ~2MB   | 几 KB               |
| 100 万 IP | ~50MB | ~100MB | 几 KB               |
| 10 亿 IP  | ❌爆炸 | ❌爆炸  | ✅稳如狗（只存前 K） |

------

**Count-Min Sketch 的原理是什么**

Count-Min Sketch（简称 CMS）是一种概率型数据结构，用于高效统计元素频率，在空间占用极小的前提下，能提供近似的计数结果。可以理解为一个二维数组结构：

```
CMS: d 行 × w 列 的二维数组（初始化为 0）

    ┌────────────┐
h1: │ 0  0  0  0 │
h2: │ 0  0  0  0 │
h3: │ 0  0  0  0 │
    └────────────┘
```

- 有 `d` 个不同的哈希函数：`h1, h2, ..., hd`
- 每个哈希函数将输入元素映射到 `[0, w-1]` 中某个列

**当插入时：**

例如要统计 `"1.2.3.4"` 的出现频率：

```go
for i := 0; i < d; i++ {
    index := hash_i("1.2.3.4") % w
    CMS[i][index] += 1
}
```

每行哈希到的位置 +1，累计频率。

**查询频率（Estimate）：**

```go
min = ∞
for i := 0; i < d; i++ {
    index := hash_i("1.2.3.4") % w
    min = min(min, CMS[i][index])
}
return min
```

为什么取最小值？因为可能有“哈希冲突”，有的格子被多个 IP 占了；但最小值最接近真实值。

**精度和内存控制:**

参数：

- `w = ceil(e / ε)`：控制误差率 ε（如 0.01）
- `d = ceil(ln(1/δ))`：控制置信度 δ（如 99%）

例如：

- 误差 ≤ 实际值 1%
- 置信度 ≥ 99%
- 只需要几 KB 的内存！

**在 `RedisBloom` 模块中，Count-Min Sketch 可用于：**

| 应用场景        | 命令示例                        |
| --------------- | ------------------------------- |
| 创建结构        | `CMS.INITBYDIM key width depth` |
| 加频率          | `CMS.INCRBY key item count`     |
| 查询频率        | `CMS.QUERY key item1 item2 ...` |
| 合并多个 sketch | `CMS.MERGE`                     |

| 优点                       | 缺点                     |
| -------------------------- | ------------------------ |
| ✅ 内存小，适合大数据场景   | ❌ 近似值，可能略高不准确 |
| ✅ 查询和插入都是 O(d) 时间 | ❌ 无法删除元素           |
| ✅ 支持并发场景             | ❌ 误差会随冲突增多而变大 |