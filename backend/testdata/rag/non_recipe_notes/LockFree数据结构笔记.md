# Lock-Free 数据结构笔记

## 为什么需要 Lock-Free

传统的锁（Mutex）在并发场景下有几个问题：
- **优先级反转**：低优先级线程持有锁时被抢占，高优先级线程等待
- **死锁**：多个线程互相等待对方释放锁
- **锁竞争**：大量线程争抢同一把锁，吞吐量下降

Lock-Free 数据结构通过原子操作而非锁来保证线程安全，避免上述问题。

## CAS 操作

CAS（Compare-And-Swap）是 Lock-Free 编程的基础原语：

```go
// CAS 的语义：如果 addr 的当前值等于 old，则设置为 new 并返回 true
// Go 通过 sync/atomic 包提供
import "sync/atomic"

var value int64

// 原子读取
current := atomic.LoadInt64(&value)

// 原子 CAS：如果 value 还是 current，就更新为 newValue
swapped := atomic.CompareAndSwapInt64(&value, current, newValue)
```

### CAS 的 ABA 问题

| 时间 | 线程1 | 线程2 |
|------|-------|-------|
| T1 | 读取 value = A | |
| T2 | | 修改 A → B |
| T3 | | 修改 B → A |
| T4 | CAS(A→C) 成功，但 value 在 T2/T3 已经被改过！ | |

解决 ABA 问题的方法：
- 带版本号/标记的 CAS（Go 的 `atomic.Value`）
- Hazard Pointer（延迟回收）
- 使用 `atomic.Pointer` 替代原始指针比较

## 无锁队列的实现

```go
type LockFreeQueue struct {
    head atomic.Pointer[node]
    tail atomic.Pointer[node]
}

type node struct {
    value int
    next  atomic.Pointer[node]
}

func (q *LockFreeQueue) Enqueue(v int) {
    n := &node{value: v}
    for {
        tail := q.tail.Load()
        next := tail.next.Load()
        if tail != q.tail.Load() {
            continue // tail 已经变了，重试
        }
        if next != nil {
            // tail 落后了，帮它推进
            q.tail.CompareAndSwap(tail, next)
            continue
        }
        if tail.next.CompareAndSwap(next, n) {
            q.tail.CompareAndSwap(tail, n)
            return
        }
    }
}
```

这个实现基于 Michael-Scott 算法，关键设计：
- Enqueue 先 CAS 设置 tail.next，成功后 CAS 更新 tail
- 如果发现 tail 落后（tail.next 已经有值），先帮它推进再继续自己的操作
- Dequeue 同理，先 CAS 更新 head，成功后才读取值

## Go 标准库中的 Lock-Free 实现

| 类型 | 实现方式 | 适用场景 |
|------|----------|----------|
| `sync.Mutex` | CAS + 信号量（有锁，但是轻量的） | 通用互斥 |
| `sync/atomic.Int64` | 硬件原子指令 | 计数器、标志位 |
| `sync/atomic.Pointer` | 原子指针操作 | 无锁数据结构 |
| `sync.Map` | 读写分离 + 原子操作 | 读多写少的 map |
| `sync.Pool` | 无锁的临时对象池 | 减少 GC 压力 |
| channel | 内部使用 ring buffer + 原子操作 | goroutine 间通信 |

## 什么时候不该用 Lock-Free

Lock-Free 不是银弹，以下场景不适合：

1. **复杂的状态更新**：多个变量需要原子更新时，Mutex 简单且正确
2. **争用不激烈的场景**：Mutex 在无竞争时开销极低（Linux futex 的 fast path）
3. **需要调试的场景**：Lock-Free 代码的 bug 极难复现和调试
4. **开发速度优先**：Lock-Free 算法设计和验证成本高

实际建议：先用 Mutex，pprof 证明锁是瓶颈再换 Lock-Free。大多数场景下，好的数据访问模式（减少共享、分片锁、读多写少用 RWMutex）比 Lock-Free 更重要。
