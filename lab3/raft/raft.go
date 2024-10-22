package raft

//
// this is an outline of the API that raft must expose to
// the service (or tester). see comments below for
// each of these functions for more details.
//
// rf = Make(...)
//   create a new Raft server.
// rf.Start(command interface{}) (index, term, isleader)
//   start agreement on a new log entry
// rf.GetState() (term, isLeader)
//   ask a Raft for its current term, and whether it thinks it is leader
// ApplyMsg
//   each time a new entry is committed to the log, each Raft peer
//   should send an ApplyMsg to the service (or tester)
//   in the same server.
//

import (
	"fmt"
	"math/rand"
	//	"bytes"
	"sync"
	"sync/atomic"
	"time"

	//	"6.824/labgob"
	"6.824/labrpc"
)

// as each Raft peer becomes aware that successive log entries are
// committed, the peer should send an ApplyMsg to the service (or
// tester) on the same server, via the applyCh passed to Make(). set
// CommandValid to true to indicate that the ApplyMsg contains a newly
// committed log entry.
type ApplyMsg struct {
	CommandValid bool
	Command      interface{}
	CommandIndex int
}

type LogEntry struct {
	Term    int
	Command interface{}
}

type State int32

const (
	FOLLOWER State = iota
	CANDIDATE
	LEADER
)

const (
	HeartbeatInterval  = 120 * time.Millisecond
	MinElectionTimeout = 300 * time.Millisecond
	MaxElectionTimeout = 600 * time.Millisecond
)

// A Go object implementing a single Raft peer.
type Raft struct {
	mu        sync.Mutex          // Lock to protect shared access to this peer's state
	peers     []*labrpc.ClientEnd // RPC end points of all peers
	persister *Persister          // Object to hold this peer's persisted state
	me        int                 // this peer's index into peers[]
	dead      int32               // set by Kill()

	// Your data here (3A, 3B, 3C).
	// Look at the paper's Figure 2 for a description of what
	// state a Raft server must maintain.

	// 3A - state
	currentTerm int
	votedFor    int
	log         []LogEntry

	// 3A - volatile state
	commitIndex int
	lastApplied int
	state       State

	// 3A - volatile state on leaders
	nextIndex  []int
	matchIndex []int

	// 3A - others
	lastHeartbeat   time.Time
	electionTimeout time.Duration
	validRequest    chan struct{}
	//totalVotes      int
}

// return currentTerm and whether this server
// believes it is the leader.
func (rf *Raft) GetState() (int, bool) {
	rf.mu.Lock()
	defer rf.mu.Unlock()
	//var term int
	//var isleader bool
	// Your code here (3A).
	return rf.currentTerm, rf.state == LEADER
}

// save Raft's persistent state to stable storage,
// where it can later be retrieved after a crash and restart.
// see paper's Figure 2 for a description of what should be persistent.
func (rf *Raft) persist() {
	// Your code here (3C).
	// Example:
	// w := new(bytes.Buffer)
	// e := labgob.NewEncoder(w)
	// e.Encode(rf.xxx)
	// e.Encode(rf.yyy)
	// data := w.Bytes()
	// rf.persister.SaveRaftState(data)
}

// restore previously persisted state.
func (rf *Raft) readPersist(data []byte) {
	if data == nil || len(data) < 1 { // bootstrap without any state?
		return
	}
	// Your code here (3C).
	// Example:
	// r := bytes.NewBuffer(data)
	// d := labgob.NewDecoder(r)
	// var xxx
	// var yyy
	// if d.Decode(&xxx) != nil ||
	//    d.Decode(&yyy) != nil {
	//   error...
	// } else {
	//   rf.xxx = xxx
	//   rf.yyy = yyy
	// }
}

// =========== FUNCTIONS FOR 3A ===========

func (rf *Raft) ShouldStartElection() bool {
	// should start an election if we haven't received a heartbeat after some predefined interval
	//return time.Now().UnixMilli()-rf.lastHeartbeat > int64(rf.electionTimeout)
	return time.Since(rf.lastHeartbeat) > rf.electionTimeout
}

func (rf *Raft) resetElectionTimeout() {
	select {
	case rf.validRequest <- struct{}{}:
	default:
		// Channel is full, no need to reset
	}
	rf.lastHeartbeat = time.Now()
	rf.electionTimeout = time.Duration(rand.Intn(int(MaxElectionTimeout-MinElectionTimeout)) + int(MinElectionTimeout))
}

func (rf *Raft) getLastLogIndex() int {
	return len(rf.log) - 1
}
func (rf *Raft) StartElection() {
	rf.mu.Lock()
	if rf.state != CANDIDATE {
		rf.mu.Unlock()
		return
	}
	var votesReceived atomic.Int32
	votesReceived.Add(1)

	//fmt.Printf("[Term %d][Node %d] Starting election 🗳️\n", rf.currentTerm, rf.me)
	rf.debugLog(fmt.Sprintf("Starting election 🗳️"))
	currentTerm := rf.currentTerm
	candidateId := rf.me
	lastLogIndex := rf.getLastLogIndex()
	lastLogTerm := rf.log[len(rf.log)-1].Term
	rf.mu.Unlock()
	requestArgs := RequestVoteArgs{
		Term:         currentTerm,
		CandidateId:  candidateId,
		LastLogIndex: lastLogIndex,
		LastLogTerm:  lastLogTerm,
	}

	for peer := range rf.peers {
		if peer != rf.me {
			go func(peer int) {
				var requestReply RequestVoteReply
				res := rf.sendRequestVote(peer, &requestArgs, &requestReply)
				if res {
					rf.mu.Lock()
					defer rf.mu.Unlock()
					if rf.currentTerm != currentTerm || rf.state != CANDIDATE {
						return // term has changed or no longer a candidate
					}
					//fmt.Printf("[Term %d][Node %d] RequestVote to %d returned %t\n", rf.currentTerm, rf.me, peer, requestReply.VoteGranted)
					rf.debugLog(fmt.Sprintf("RequestVote to %d returned %t", peer, requestReply.VoteGranted))
					if requestReply.VoteGranted {
						votesReceived.Add(1)
						if int(votesReceived.Load()) > len(rf.peers)/2 {
							//fmt.Printf("[Term %d][Node %d] Node \033[1m%d\033[0m has won the election for term %d\n", rf.currentTerm, rf.me, rf.me, rf.currentTerm)
							rf.debugLog(fmt.Sprintf("Node \033[1m%d\033[0m has won the election for term %d", rf.me, rf.currentTerm))
							rf.makeLeader()
						}
					} else if requestReply.Term > rf.currentTerm {
						rf.makeFollower(requestReply.Term)
					}
				}
			}(peer)
		}
	}
}

// 3A - Auxiliary functions

func (rf *Raft) getLogEntry(index int) LogEntry {
	return rf.log[index]
}

func (rf *Raft) getPrevLogIndex(index int) int {
	return rf.nextIndex[index] - 1
}

func (rf *Raft) isLogUpToDate(lastLogIndex, lastLogTerm int) bool {
	if lastLogTerm > rf.log[len(rf.log)-1].Term {
		return true
	} else if lastLogTerm == rf.log[len(rf.log)-1].Term {
		return lastLogIndex >= len(rf.log)
	} else {
		return false
	}

}

// example RequestVote RPC arguments structure.
// field names must start with capital letters!
type RequestVoteArgs struct {
	// Your data here (3A, 3B).
	Term         int
	CandidateId  int
	LastLogIndex int
	LastLogTerm  int
}

// example RequestVote RPC reply structure.
// field names must start with capital letters!
type RequestVoteReply struct {
	// Your data here (3A).
	Term        int
	VoteGranted bool
}

// example RequestVote RPC handler.
func (rf *Raft) RequestVote(args *RequestVoteArgs, reply *RequestVoteReply) {
	// Your code here (3A, 3B).
	rf.mu.Lock()
	defer rf.mu.Unlock()
	rf.resetElectionTimeout()

	rf.debugLog(fmt.Sprintf("RequestVote from %d. Request on term %d", args.CandidateId, args.Term))
	reply.Term = rf.currentTerm

	// check if term < currentTerm -> indicates lagging behind
	if args.Term < rf.currentTerm {
		//fmt.Printf("[args.")
		reply.VoteGranted = false
		return
	} else if args.Term > rf.currentTerm {
		if rf.state != FOLLOWER {
			rf.makeFollower(args.Term)
		}
		rf.currentTerm = args.Term
		reply.VoteGranted = true
		return
	} else {
		// equals
		reply.VoteGranted = false
		return
	}

	//if (rf.votedFor == -1 || rf.votedFor == args.CandidateId) && rf.isLogUpToDate(args.LastLogIndex, args.LastLogTerm) {
	//	reply.Term = rf.currentTerm
	//	reply.VoteGranted = true
	//	rf.votedFor = args.CandidateId
	//	rf.resetElectionTimeout()
	//	return
	//}

	// TODO: maybe check if candidate term is greater than current term -> indicates this server is lagging behind and we should make it a follower

	//fmt.Printf("[Term %d][Node %d] Voted for %d and candidate is %d\n", rf.currentTerm, rf.me, rf.votedFor, args.CandidateId)
	//
	//return
}

// example code to send a RequestVote RPC to a server.
// server is the index of the target server in rf.peers[].
// expects RPC arguments in args.
// fills in *reply with RPC reply, so caller should
// pass &reply.
// the types of the args and reply passed to Call() must be
// the same as the types of the arguments declared in the
// handler function (including whether they are pointers).
//
// The labrpc package simulates a lossy network, in which servers
// may be unreachable, and in which requests and replies may be lost.
// Call() sends a request and waits for a reply. If a reply arrives
// within a timeout interval, Call() returns true; otherwise
// Call() returns false. Thus Call() may not return for a while.
// A false return can be caused by a dead server, a live server that
// can't be reached, a lost request, or a lost reply.
//
// Call() is guaranteed to return (perhaps after a delay) *except* if the
// handler function on the server side does not return.  Thus there
// is no need to implement your own timeouts around Call().
//
// look at the comments in ../labrpc/labrpc.go for more details.
//
// if you're having trouble getting RPC to work, check that you've
// capitalized all field names in structs passed over RPC, and
// that the caller passes the address of the reply struct with &, not
// the struct itself.
func (rf *Raft) sendRequestVote(server int, args *RequestVoteArgs, reply *RequestVoteReply) bool {
	ok := rf.peers[server].Call("Raft.RequestVote", args, reply)
	return ok
}

type AppendEntriesArgs struct {
	Term         int
	LeaderId     int
	PrevLogIndex int
	PrevLogTerm  int
	Entries      []LogEntry
	LeaderCommit int
}

type AppendEntriesReply struct {
	Term    int
	Success bool
}

func (rf *Raft) debugLog(s string) {
	return
	fmt.Printf("[Term %d][Node %d] %s", rf.currentTerm, rf.me, s)
}

// should only be called as a follower
func (rf *Raft) AppendEntries(args *AppendEntriesArgs, reply *AppendEntriesReply) {
	//fmt.Printf("[Term %d][Node %d] AppendEntries from %d, at term %d\n", rf.currentTerm, rf.me, args.LeaderId, args.Term)
	rf.mu.Lock()
	defer rf.mu.Unlock()
	// reset the election timeout bc we saw some heartbeat from the server
	rf.resetElectionTimeout()
	rf.debugLog(fmt.Sprintf("AFTER LOCK AppendEntries from %d, at term %d\n", args.LeaderId, args.Term))
	reply.Term = rf.currentTerm

	if args.Term < rf.currentTerm { // Input term < current term, means requester is behind
		reply.Success = false
		return
	} else if args.Term >= rf.currentTerm { // -----H-------
		if rf.state != FOLLOWER { // -----HERE or later
			rf.makeFollower(args.Term)
		} else {
			rf.currentTerm = args.Term

		}
		reply.Success = true
	}
}

func (rf *Raft) sendAppendEntries(server int, args *AppendEntriesArgs, reply *AppendEntriesReply) bool {
	return rf.peers[server].Call("Raft.AppendEntries", args, reply)
}

// the service using Raft (e.g. a k/v server) wants to start
// agreement on the next command to be appended to Raft's log. if this
// server isn't the leader, returns false. otherwise start the
// agreement and return immediately. there is no guarantee that this
// command will ever be committed to the Raft log, since the leader
// may fail or lose an election. even if the Raft instance has been killed,
// this function should return gracefully.
//
// the first return value is the index that the command will appear at
// if it's ever committed. the second return value is the current
// term. the third return value is true if this server believes it is
// the leader.
func (rf *Raft) Start(command interface{}) (int, int, bool) {
	index := -1
	term := -1
	isLeader := true

	// Your code here (3B).

	return index, term, isLeader
}

// the tester doesn't halt goroutines created by Raft after each test,
// but it does call the Kill() method. your code can use killed() to
// check whether Kill() has been called. the use of atomic avoids the
// need for a lock.
//
// the issue is that long-running goroutines use memory and may chew
// up CPU time, perhaps causing later tests to fail and generating
// confusing debug output. any goroutine with a long-running loop
// should call killed() to check whether it should stop.
func (rf *Raft) Kill() {
	atomic.StoreInt32(&rf.dead, 1)
	// Your code here, if desired.
}

func (rf *Raft) killed() bool {
	z := atomic.LoadInt32(&rf.dead)
	return z == 1
}

// ======= 3A - Election-related functions =======

func (rf *Raft) makeCandidate() {
	rf.mu.Lock()
	//if rf.state == LEADER {
	//	return
	//}
	rf.resetElectionTimeout()
	rf.state = CANDIDATE
	rf.currentTerm++
	rf.votedFor = rf.me
	//rf.totalVotes = 1
	rf.mu.Unlock()
	go rf.StartElection()
}

func (rf *Raft) makeFollower(term int) {
	rf.resetElectionTimeout()
	rf.state = FOLLOWER
	rf.votedFor = -1
	//rf.totalVotes = 0
	rf.currentTerm = term
}

func (rf *Raft) makeLeader() {
	if rf.state != CANDIDATE {
		return
	}
	rf.resetElectionTimeout()
	rf.state = LEADER
	rf.nextIndex = make([]int, len(rf.peers))
	rf.debugLog(fmt.Sprintf("is now the leader"))

	//for i := range rf.peers {
	//	rf.nextIndex[i] = len(rf.log)
	//	rf.matchIndex[i] = 0
	//}

	go rf.leaderAppendEntries()
}

// runs once, sends an AppendEntries RPC to all peers
func (rf *Raft) leaderAppendEntries() {
	// should only run during a leader
	rf.mu.Lock()
	if rf.state != LEADER {
		rf.mu.Unlock()
		return
	}
	args := AppendEntriesArgs{
		Term:     rf.currentTerm,
		LeaderId: rf.me,
	}
	rf.mu.Unlock()
	//println("Node ", rf.me, " is sending heartbeats for term ", rf.currentTerm)
	rf.debugLog(fmt.Sprintf("Sending heartbeats❤️❤️ of length %d", len(rf.peers)-1))

	for peer := range rf.peers {
		if peer != rf.me {
			go func(peer int) {
				var reply AppendEntriesReply
				res := rf.sendAppendEntries(peer, &args, &reply)
				if res {
					rf.mu.Lock()
					defer rf.mu.Unlock()
					if reply.Term > rf.currentTerm {
						rf.debugLog(fmt.Sprintf("Making %d a follower due to lagged term", peer))
						rf.makeFollower(reply.Term)
					}
				}
			}(peer)
		}
	}
}

// The ticker go routine starts a new election if this peer hasn't received
// heartsbeats recently.
func (rf *Raft) ticker() {
	for rf.killed() == false {
		rf.mu.Lock()
		state := rf.state
		rf.mu.Unlock()

		//delayTime := time.Duration(rand.Intn(MaxElectionTimeout - MinElectionTimeout)+MinElectionTimeout) * time.Millisecond
		delayTime := time.Duration(rand.Intn(int(MaxElectionTimeout-MinElectionTimeout)) + int(MinElectionTimeout))
		switch state {
		case FOLLOWER, CANDIDATE:
			{
				select {
				case <-rf.validRequest:
					// do nothing and go to next iteration
				case <-time.After(delayTime):
					rf.makeCandidate()
				}
			}
		case LEADER:
			{
				go rf.leaderAppendEntries()
				time.Sleep(HeartbeatInterval)
			}
		}

		// Your code here to check if a leader election should
		// be started and to randomize sleeping time using
		// time.Sleep().
		//time.Sleep(10 * time.Millisecond)
		//rf.mu.Lock()
		//if rf.state == FOLLOWER && rf.ShouldStartElection() {
		//	rf.mu.Unlock()
		//	rf.StartElection()
		//} else {
		//	rf.mu.Unlock()
		//}

	}
}

// the service or tester wants to create a Raft server. the ports
// of all the Raft servers (including this one) are in peers[]. this
// server's port is peers[me]. all the servers' peers[] arrays
// have the same order. persister is a place for this server to
// save its persistent state, and also initially holds the most
// recent saved state, if any. applyCh is a channel on which the
// tester or service expects Raft to send ApplyMsg messages.
// Make() must return quickly, so it should start goroutines
// for any long-running work.
func Make(peers []*labrpc.ClientEnd, me int,
	persister *Persister, applyCh chan ApplyMsg) *Raft {
	rf := &Raft{}
	rf.peers = peers
	rf.persister = persister
	rf.me = me

	// Your initialization code here (3A, 3B, 3C).
	rf.state = FOLLOWER
	rf.votedFor = -1
	rf.currentTerm = 0
	rf.commitIndex = 0
	rf.lastApplied = 0
	rf.nextIndex = make([]int, len(peers))
	rf.matchIndex = make([]int, len(peers))
	rf.validRequest = make(chan struct{}, 1)
	rf.resetElectionTimeout()
	rf.log = append(rf.log, LogEntry{Term: 0})

	// initialize from state persisted before a crash
	rf.readPersist(persister.ReadRaftState())

	println("Starting server ", rf.me, " with term ", rf.currentTerm)

	// start ticker goroutine to start elections
	go rf.ticker()

	return rf
}
