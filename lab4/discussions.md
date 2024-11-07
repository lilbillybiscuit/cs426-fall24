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

