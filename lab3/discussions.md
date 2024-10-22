## Lab 2 - Raft Part 1

## 3A-2

### (1) - scenario where leader election fails

Consider the following scenario (that we aim to achieve by setting the desired timeouts), presented in chronological order. Assume each step is possible to achieve, which we will concretely describe by timestamps later.

1. All notes start as followers with the same term (eg. term 0, the start of the program)
2. Node A, B, and C start the election at the exact same time. They all increase their respective terms to 1, and send RequestVotes to the other two nodes
3. 6 RequestVotes are sent. For example, for A RequestVote -> B, B will be on the same term as A, so it will reject the request. Other RequestVotes are similarily handled, so none of the RequestVotes succeed.
4. Because none of them succeed, after an election timeout, there will not have been a leader elected. Therefore, the cycle starts over again.

To do this, we can control rand and time by setting the timeout to the same value on all nodes (or having rand generate the same number every time)

### (2) - In practice
This is not a major concern because Raft's design takes into account elements of randomness that reduces the probability of this happening. For example, the following aspects help to get around the theoretical possibility:
1.	Using randomized election timeouts: this reduces the likelihood of simultaneous elections, making it unlikely that nodes will continuously split votes
2.	Real-world network conditions are variable but not controlled by an adversary (even things such as physics, noise, etc are hard to control), meaning that eventually, messages will be delivered in a way that allows a leader to be elected.
3.	We can implement logic in nodes to adjust their behavior based on observed network conditions, such as increasing timeouts if elections consistently fail.
4.	And even in practice, it's unlikely that an adversary can indefinitely control message delivery and timing to prevent leader election, as multiple security boundaries would likely need to be broken first


## ExtraCredit1