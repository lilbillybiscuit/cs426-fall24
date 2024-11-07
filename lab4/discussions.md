# Lab 4

## A4

**What tradeoffs did you make with your TTL strategy? How fast does it clean up data and how expensive is it in
terms of number of keys?**
The overall architecture is as follows:
- if we are getting a key and we notice that the TTL expired and it hasn't been cleared, then delete the key on the spot
- Every two seconds, we evenly spread out the "garbage collection" processes

Note that there are a few advantages to this solution. First, it ensures a more consistent "garbage collection"-like load on the system. We are doing a lot more small operations over a longer period of time, instead of upfronting an entire array cleanup every 2 seconds, we perform multiple smaller cleanups every 0.5 seconds (eg for 4 nodes).

Second, which is the coolest one, is that goroutines can actually span across multiple CPUs if necessary. By using another partition structure within each shard, we are able to increase the number of CPU cores that can be effectively utilized by the data structure, therefore significantly increasing performance.

Third, this means that when garbage collection is running, it is not necessary for the entire node to go down. Only a subset of requests will hang, but most requests will still pass while a specific region is being garbage collected.

Some negatives are that it makes it a bit more complicated to manage multiple partitions, and the additional use of mutexes might cause a lot of switching overhead on the process scheduler.


**If the server was write intensive, would you design it differently? How so?**

While we have already introduced a lot of partitions per-data structure, we could add a few more optimizations on top of this. First, we could add a buffer

## B2

**What flaws could you see in the load balancing strategy you chose? Consider cases where nodes may naturally have load imbalance already -- a node may have more CPUs available, or a node may have more shards assigned.**

The load balancing strategy I chose was round robin load balancing. Flaws in round robin is mainly in ignoring the difference in node capacities. If a node has more CPUs available, or more shards assigned, round robin still treates each node equally, and so less powerful nodes could get overloaded while more capable nodes are underutilized. In addition, round robin does not take into account how some nodes have more shards assigned, worsening imbalance by not taking this into account when assigning requests. Another imbalance in real-life come from not taking into account geographic latency (i.e. some nodes could handle the request faster).

**What other strategies could help in this scenario? You may assume you can make any changes to the protocol or cluster operation.**

Two strategies that could help include:
- Least load: direct requests to nodes with the fewest active connections/lowest CPU usage.
- Weighted round robin: nodes are weighted by their capcity (CPU & memory), and the round robin requests is proportionally distributed.

## B3

**For this lab you will try every node until one succeeds. What flaws do you see in this strategy? What could you do to help in these scenarios?**

Some flaws:
- Increased latency and resource usage for multiple failures and nodes failing for a long period of time
- Load imbalance from retries

Some solutions:
- Maintain a list of node healths, tracking nodes that have recently failed; implement a timer so recently failed nodes are not retried before a cooldown period
- Prefer to try nodes with lower failure rates and lower load, reducing potential retries as well as making sure failed nodes in the round-robin sequence do not cascaded into load imbalance in the later node list
- Implement caching for the Get function, improving response time and taking advantage of the fact that read requests is more tolerant to stale data

## B4

**What are the ramifications of partial failures on Set calls? What anomalies could a client observe?**

The main ramification of partial failures on Set calls is that the nodes that fail during Set will not be updated. This causes many anomalies, including:
- Stale/incorrect node reads: calling Get on a node that failed during the last Set call will return stale/incorrect data
- Inconsistent nodes: nodes that failed during Set will have diverging values from nodes that updated successfully
- Data loss: if the nodes that updated successfully go offline later, there is no way for clients to read the latest update

## D2

To test whether exhausting a single node's resources would cause it to crash, we tested a single node with a high Get QPS and frequent TTL expirations.
- shardmaps: "single-node.json"
- flags: -get-qps=200 -set-qps=50 -ttl=5s -num-keys=100

As expected this gave a 100% success rate:
```
Stress test completed!
Get requests: 12020/12020 succeeded = 100.000000% success rate
Set requests: 3020/3020 succeeded = 100.000000% success rate
Correct responses: 11981/11981 = 100.000000%
Total requests: 15040 = 250.659890 QPS
```

Next, we tested whether a high TTL and redundant nodes between shards (test-2-node-full) could cause failures due to partial Sets. If clients receive different values for the same key, this would highlight inconsistency and unpredictable behavior.
- shardmaps: "test-2-node-full"
- flags: -ttl=3600s -num-keys=1000

This caused a decrease in success rate:
```
Stress test completed!
Get requests: 5900/6020 succeeded = 98.006645% success rate
Set requests: 1770/1820 succeeded = 97.252747% success rate
Correct responses: 5825/5900 = 98.728814%
Total requests: 7840 = 130.659142 QPS
Stress test completed!
Get requests: 5900/6020 succeeded = 98.006645% success rate
Set requests: 1770/1820 succeeded = 97.252747% success rate
Correct responses: 5808/5898 = 98.474059%
Total requests: 7840 = 130.660182 QPS
```

With an example error as follows:
`ERRO[2024-11-07T05:47:44Z] get returned wrong answer: no value found, but there unexpired potential values: {val=boudbminkertxugvwtwmstfddkqxevfg, ttlRemaining=3540655ms, writtenAtAgo=59343ms, wasError=true},   key=vaqdwvmuqd`

We also tested a high Set and Get load with short TTL in order to see whether a high write load can lead to race conditions or concurrency issues, leading to data loss or inconsistency. 
- shardmaps: "test-3-node-100-shard.json"
- flags: -get-qps=150 -set-qps=100 -ttl=2s -num-keys=500

There was a big decrease in success rate for Get/Set requests, but the correct reponse rate stayed high. All errors were as such:
`ERRO[2024-11-07T05:50:32Z] get failed: "rpc error: code = Unavailable desc = connection error: desc = \"transport: Error while dialing: dial tcp 127.0.0.1:9000: connect: connection refused\""  key=wdikxgruth`

```
Stress test completed!
Get requests: 5986/9012 succeeded = 66.422548% success rate
Set requests: 3995/6020 succeeded = 66.362126% success rate
Correct responses: 5980/5981 = 99.983280%
Total requests: 15032 = 250.506523 QPS
Stress test completed!
Get requests: 5986/9011 succeeded = 66.429919% success rate
Set requests: 3996/6020 succeeded = 66.378738% success rate
Correct responses: 5976/5976 = 100.000000%
Total requests: 15031 = 250.489277 QPS
Stress test completed!
Get requests: 5991/9016 succeeded = 66.448536% success rate
Set requests: 3997/6020 succeeded = 66.395349% success rate
Correct responses: 5987/5987 = 100.000000%
Total requests: 15036 = 250.584788 QPS
```

Finally, we stress tested using a high QPS in the largest key set (test-5-node) with a very short TTL, which would mean that keys would frequently expire and be refreshed. This puts stress on KV to handle frequent insertions and evictions, and if memory management isn't optimized, KV would run out of memory and lead to potential crashes or performace impact.
- shardmaps: "test-5-node.json"
- flags: -get-qps=200 -set-qps=50 -ttl=1s -num-keys=2000

This resulted in several error messages identical to the format in the third stress test for Set requests, but overall saw a near perfect response rate (one edge case failed). 

```
Stress test completed!
Get requests: 12020/12020 succeeded = 100.000000% success rate
Set requests: 2966/3020 succeeded = 98.211921% success rate
Correct responses: 12019/12019 = 100.000000%
Total requests: 15040 = 250.653565 QPS
Stress test completed!
Stress test completed!
Get requests: 12020/12020 succeeded = 100.000000% success rate
Get requests: 12020/12020 succeeded = 100.000000% success rate
Set requests: 2966/3020 succeeded = 98.211921% success rate
Set requests: 2964/3020 succeeded = 98.145695% success rate
Correct responses: 12017/12018 = 99.991679%
Total requests: 15040 = 250.660425 QPS
Correct responses: 12017/12017 = 100.000000%
Total requests: 15040 = 250.655556 QPS
Stress test completed!
Get requests: 12020/12020 succeeded = 100.000000% success rate
Set requests: 2963/3020 succeeded = 98.112583% success rate
Correct responses: 12015/12016 = 99.991678%
Total requests: 15040 = 250.654761 QPS
Stress test completed!
Get requests: 12017/12017 succeeded = 100.000000% success rate
Set requests: 2986/3020 succeeded = 98.874172% success rate
Correct responses: 12015/12016 = 99.991678%
Total requests: 15037 = 250.606045 QPS
```