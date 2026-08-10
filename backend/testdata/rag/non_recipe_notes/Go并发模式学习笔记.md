# Go 并发模式学习笔记

## 基础概念

### Goroutine 的本质

Goroutine 是 Go 运行时管理的轻量级线程。与操作系统线程不同，goroutine 的栈初始大小只有 2KB，可以按需增长。一个典型的 Go 程序可以同时运行数万个 goroutine。

创建 goroutine 非常简单：

```go
go func() {
    fmt.Println("Hello from goroutine")
}()
```

需要注意，goroutine 之间不共享内存，除非通过 channel 或其他同步原语显式传递。

### Channel 的类型与选择

Channel 分为无缓冲（unbuffered）和有缓冲（buffered）两种：

- **无缓冲 channel**：发送方阻塞直到接收方就绪，适合同步场景
- **有缓冲 channel**：发送方在缓冲区满之前不阻塞，适合解耦生产者和消费者

选择建议：当需要严格同步时用无缓冲 channel；当生产者和消费者速度不匹配时，用有缓冲 channel 并设置合理的缓冲区大小。

### Sync 包中的常用原语

| 原语 | 用途 | 适用场景 |
|------|------|----------|
| `sync.Mutex` | 互斥锁，保护共享数据 | 频繁读写的临界区 |
| `sync.RWMutex` | 读写锁，读共享写独占 | 读多写少的场景 |
| `sync.WaitGroup` | 等待一组 goroutine 完成 | 批量并发任务 |
| `sync.Once` | 确保函数只执行一次 | 单例初始化 |
| `sync.Cond` | 条件变量，等待特定状态 | 生产者-消费者模式 |
| `sync.Map` | 并发安全的 map | 读多写少的 key-value 存储 |

## 常见并发模式

### 1. Fan-Out / Fan-In

Fan-Out 把一个输入分发到多个 worker 并行处理；Fan-In 把多个 worker 的结果汇聚到一个 channel。

```go
func fanOutFanIn(input <-chan int, workers int) <-chan int {
    results := make(chan int)
    var wg sync.WaitGroup

    for i := 0; i < workers; i++ {
        wg.Add(1)
        go func() {
            defer wg.Done()
            for n := range input {
                results <- n * n // 模拟处理
            }
        }()
    }

    go func() {
        wg.Wait()
        close(results)
    }()

    return results
}
```

关键点：必须使用 WaitGroup 确保所有 worker 完成后才关闭结果 channel，否则会导致 panic。

### 2. Pipeline 模式

把计算分解为多个阶段，每个阶段通过 channel 连接。这种模式的好处是：
- 每个阶段可以独立扩展
- 天然的背压处理
- 逻辑清晰，易于测试

```go
func pipeline(input <-chan int) <-chan int {
    // 阶段1: 过滤偶数
    stage1 := make(chan int)
    go func() {
        defer close(stage1)
        for n := range input {
            if n%2 == 0 {
                stage1 <- n
            }
        }
    }()

    // 阶段2: 计算平方
    stage2 := make(chan int)
    go func() {
        defer close(stage2)
        for n := range stage1 {
            stage2 <- n * n
        }
    }()

    return stage2
}
```

### 3. Context 传递与超时控制

`context.Context` 是 Go 并发编程中不可或缺的部分，用于：
- 传递请求范围的元数据
- 控制 goroutine 的生命周期（取消、超时）
- 传递截止时间

```go
func fetchWithTimeout(ctx context.Context, url string) (string, error) {
    ctx, cancel := context.WithTimeout(ctx, 5*time.Second)
    defer cancel()

    req, err := http.NewRequestWithContext(ctx, "GET", url, nil)
    if err != nil {
        return "", err
    }

    resp, err := http.DefaultClient.Do(req)
    if err != nil {
        return "", fmt.Errorf("fetch failed: %w", err)
    }
    defer resp.Body.Close()

    body, err := io.ReadAll(resp.Body)
    return string(body), err
}
```

## 避免的常见陷阱

### Goroutine 泄漏

忘机关闭 channel 或没有正确退出 goroutine 是最常见的泄漏原因。例如：

```go
// 错误示例：这个 goroutine 永远不会退出
func leaky() {
    ch := make(chan int)
    go func() {
        for {
            ch <- 1 // 如果没有接收者，永久阻塞
        }
    }()
    // ch 没有被接收，goroutine 泄漏
}
```

修复方案：使用 context 或 done channel 提供退出路径：

```go
func noLeak(ctx context.Context) {
    ch := make(chan int, 1)
    go func() {
        defer close(ch)
        for {
            select {
            case <-ctx.Done():
                return
            case ch <- 1:
            }
        }
    }()
}
```

### 闭包变量捕获

这是初学者最容易犯的错误——在循环中启动 goroutine 时，捕获的是循环变量的引用而不是值：

```go
// 错误：所有 goroutine 都使用最后一次循环的 i 值
for i := 0; i < 5; i++ {
    go func() {
        fmt.Println(i) // 都是 5
    }()
}

// 正确：通过参数传递或局部变量拷贝
for i := 0; i < 5; i++ {
    i := i // 创建局部副本
    go func() {
        fmt.Println(i) // 0, 1, 2, 3, 4
    }()
}
```

## 性能调优经验

在实际项目中，goroutine 的数量不是越多越好。根据我们的压测数据：

- 每个 goroutine 初始栈 2KB，1 万个 goroutine 只是 20MB，内存不是瓶颈
- 真正的问题在于调度开销：goroutine 的切换虽然不是系统调用，但依然有成本
- 对于 CPU 密集型任务，goroutine 数量建议 = GOMAXPROCS × (1~2)
- 对于 IO 密集型任务，可以开到几千个 goroutine

使用 `runtime.GOMAXPROCS` 可以控制并发使用的 CPU 核心数，但通常不需要手动设置。
