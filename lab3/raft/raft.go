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
	"6.824/labgob"
	"bytes"
	"fmt"
	"log"
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

type RequestVoteArgs struct {
	// Your data here (3A, 3B).
	Term         int
	CandidateId  int
	LastLogIndex int
	LastLogTerm  int
}
type RequestVoteReply struct {
	// Your data here (3A).
	Term        int
	VoteGranted bool
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
	Term             int
	Success          bool
	ConflictingTerm  int
	ConflictingIndex int
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

// TimedMutex is a custom mutex that tracks how long it has been locked.
type TimedMutex struct {
	mu       sync.Mutex
	lockTime time.Time
	isLocked bool
}

// Lock locks the mutex and records the time it was locked.
func (tm *TimedMutex) Lock() {
	tm.mu.Lock()
	tm.lockTime = time.Now()
	tm.isLocked = true
}

func (tm *TimedMutex) Unlock() {
	tm.isLocked = false
	tm.mu.Unlock()
}

// MonitorLock checks if the mutex is locked for longer than the warning time.
func (tm *TimedMutex) MonitorLock() {
	for {
		time.Sleep(4 * time.Millisecond) // Check every 10ms
		if tm.isLocked && time.Since(tm.lockTime) > 10*time.Millisecond {
			//fmt.Printf("Warning: Mutex has been locked for more than %v!\n", tm.warningTime)
			fmt.Printf("Warning: Mutex has been locked for more than %v!\n", time.Since(tm.lockTime))
		}
	}
}

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

	// 3B
	applyCh chan ApplyMsg
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

func (rf *Raft) sendAppendEntries(server int, args *AppendEntriesArgs, reply *AppendEntriesReply) bool {
	return rf.peers[server].Call("Raft.AppendEntries", args, reply)
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
	//Your code here (3C).
	//Example:
	w := new(bytes.Buffer)
	e := labgob.NewEncoder(w)
	e.Encode(rf.currentTerm)
	e.Encode(rf.votedFor)
	e.Encode(rf.log)
	data := w.Bytes()
	rf.persister.SaveRaftState(data)
}

// restore previously persisted state.
func (rf *Raft) readPersist(data []byte) {
	if data == nil || len(data) < 1 { // bootstrap without any state?
		return
	}
	// Your code here (3C).
	// Example:
	r := bytes.NewBuffer(data)
	d := labgob.NewDecoder(r)
	var currentTerm int
	var votedFor int
	var tlog []LogEntry
	if d.Decode(&currentTerm) != nil || d.Decode(&votedFor) != nil || d.Decode(&tlog) != nil {
		log.Fatal("Error decoding persisted state")
	} else {
		rf.mu.Lock()
		defer rf.mu.Unlock()
		rf.currentTerm = currentTerm
		rf.votedFor = votedFor
		rf.log = tlog
	}
}

// ===========  DEBUG FUNCTION  ===========

func (rf *Raft) debugLog(s string, a ...interface{}) {
	return // disable/enable
	colors := []string{
		"\033[31m",
		"\033[33m",
		"\033[32m",
		"\033[36m",
		"\033[34m",
		"\033[35m",
		"\033[31m",
		"\033[33m",
		"\033[32m",
		"\033[36m",
	}

	reset := "\033[0m"

	color := colors[rf.me%len(colors)]

	maxBarLength := 6
	term := rf.currentTerm

	inverted := "\033[7m"

	highlightLength := 0
	if term > 0 {
		highlightLength = term
		if highlightLength > maxBarLength {
			highlightLength = maxBarLength
		}
	}

	termText := fmt.Sprintf("Term %d", term)
	termBar := fmt.Sprintf("%s%s%s%s",
		inverted,
		termText[:highlightLength],
		reset,
		termText[highlightLength:],
	)
	if rf.state == LEADER {
		fmt.Printf("[%dms][%s][%sNode %d%s] %s\n",
			time.Now().UnixNano()/1e6,
			termBar,
			color,
			rf.me,
			reset,
			fmt.Sprintf(s, a...))
	} else {
		fmt.Printf("[%dms][%s][Node %s%d%s] %s\n",
			time.Now().UnixNano()/1e6,
			termBar,
			color,
			rf.me,
			reset,
			fmt.Sprintf(s, a...))
	}

}
func (rf *Raft) printLog() {
	//return
	var s string = fmt.Sprintf("LOG: ")
	//fmt.Printf("Node %d log: ", rf.me)
	//for i, entry := range rf.log {
	//	fmt.Printf("[%d: %d] ", i, entry.Command)
	//}
	//fmt.Println()
	for i, entry := range rf.log {
		if i == 0 {
			continue
		}
		s += fmt.Sprintf("[%d: %d] ", i, entry.Command)
	}
	rf.debugLog(s)
}

func (rf *Raft) printIndexes() {
	// return

	if rf.state != LEADER {
		return
	}
	rf.printLog()
	var nextIndexS string = fmt.Sprintf("Next Indexes: ")
	//for i, entry := range rf.log {
	//	if i == 0 {
	//		continue
	//	}
	//	s += fmt.Sprintf("[%d: %d] ", i, entry.Command)
	//}
	//rf.debugLog(s)
	for i, entry := range rf.nextIndex {
		nextIndexS += fmt.Sprintf("[%d: %d] ", i, entry)
	}
	rf.debugLog(nextIndexS)

	var matchIndexS string = fmt.Sprintf("Match Indexes: ")
	for i, entry := range rf.matchIndex {
		matchIndexS += fmt.Sprintf("[%d: %d] ", i, entry)
	}
	rf.debugLog(matchIndexS)

	var commitIndexS string = fmt.Sprintf("Commit Index: %d", rf.commitIndex)
	rf.debugLog(commitIndexS)

}

// =========== FUNCTIONS FOR 3A ===========

func (rf *Raft) ShouldStartElection() bool {
	// should start an election if we haven't received a heartbeat after some predefined interval
	//return time.Now().UnixMilli()-rf.lastHeartbeat > int64(rf.electionTimeout)
	if time.Since(rf.lastHeartbeat) > rf.electionTimeout {
		rf.debugLog("Should start an election, last time was %dms", rf.lastHeartbeat.UnixNano()/1e6)
	}
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

// =========== 3B - LOG FUNCTIONS ===========
func (rf *Raft) getLastLogIndex() int {
	return len(rf.log) - 1
}
func (rf *Raft) getLastLogTerm() int {
	var index int = rf.getLastLogIndex()
	if index >= 0 {
		return rf.log[index].Term
	} else {
		return -1
	}
}
func (rf *Raft) getLogEntry(index int) LogEntry {
	return rf.log[index]
}
func (rf *Raft) getPrevLogIndex(index int) int {
	return max(rf.nextIndex[index]-1, 0)
}
func (rf *Raft) getPrevLogTerm(index int) int {
	preIndex := rf.getPrevLogIndex(index)
	if preIndex >= 0 && preIndex < len(rf.log) {
		return rf.log[preIndex].Term
	} else {
		return 0
	}
}

func (rf *Raft) isOtherLogUpToDate(queryLogIndex, queryLogTerm int) bool {
	// return true if the other log is more up to date than mine (first by term, then by length if equal)
	thisLastLogTerm := rf.getLastLogTerm() // my last log term
	if queryLogTerm > thisLastLogTerm {
		return true
	} else if queryLogTerm == thisLastLogTerm { // this only works since we are guaranteed that logs with <=term-1 are equal, so just need to check entire length
		return queryLogIndex >= len(rf.log)
	} else {
		return false
	}
}

func (rf *Raft) StartElection() {
	rf.mu.Lock()
	if rf.state != CANDIDATE {
		rf.mu.Unlock()
		return
	}
	//rf.votedFor = rf.me
	rf.debugLog(fmt.Sprintf("Starting election 🗳️"))
	currentTerm := rf.currentTerm
	requestArgs := RequestVoteArgs{
		Term:         currentTerm,
		CandidateId:  rf.me,
		LastLogIndex: rf.getLastLogIndex(),
		LastLogTerm:  rf.getLastLogTerm(),
	}
	rf.mu.Unlock()

	var votesReceived atomic.Int32
	votesReceived.Add(1)

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
						rf.makeFollowerN(requestReply.Term)
					}
				}
			}(peer)
		}
	}
}

// example RequestVote RPC handler.
func (rf *Raft) RequestVote(args *RequestVoteArgs, reply *RequestVoteReply) {
	// Your code here (3A, 3B).
	rf.mu.Lock()
	defer rf.mu.Unlock()

	//rf.debugLog(fmt.Sprintf("RequestVote from %d. Request on term %d", args.CandidateId, args.Term))
	reply.Term = rf.currentTerm
	reply.VoteGranted = false

	// term too bigh -> become follower
	if args.Term > rf.currentTerm {
		rf.makeFollowerN(args.Term)
		//rf.currentTerm = args.Term
		//reply.VoteGranted = true
	} else if args.Term < rf.currentTerm { // term too low, candidate is not valid
		return
	}
	// already voted
	if rf.votedFor != -1 && rf.votedFor != args.CandidateId {
		return
	}
	// candidates last log term is too behind -> bad candidate
	if args.LastLogTerm < rf.getLastLogTerm() {
		return
	} else if args.LastLogTerm == rf.getLastLogTerm() {
		if args.LastLogIndex < rf.getLastLogIndex() {
			return
		}
	}
	rf.votedFor = args.CandidateId
	reply.VoteGranted = true
	//rf.state = FOLLOWER // TODO: fix
	rf.persist()
	rf.resetElectionTimeout()
}

func (rf *Raft) findConflict(args *AppendEntriesArgs) (success bool, term, index int) {
	// out of bounds
	if args.PrevLogIndex < 0 || args.PrevLogIndex >= len(rf.log) {
		return false, -1, len(rf.log)
	}

	conflictTerm := rf.log[args.PrevLogIndex].Term
	// terms match case
	if conflictTerm == args.PrevLogTerm {
		return true, 0, 0
	}

	// Find first occurrence of conflict term
	for i, entry := range rf.log {
		if entry.Term == conflictTerm {
			return false, conflictTerm, i
		}
	}

	return false, conflictTerm, len(rf.log)
}

func (rf *Raft) packageAppendEntryReply(reply *AppendEntriesReply, success bool, conflictingTerm, conflictingIndex int) {
	reply.Term = rf.currentTerm
	reply.Success = success
	reply.ConflictingTerm = conflictingTerm
	reply.ConflictingIndex = conflictingIndex
}

// should only be called as a follower
func (rf *Raft) AppendEntries(args *AppendEntriesArgs, reply *AppendEntriesReply) {
	//rf.debugLog("BEFORE LOCK AppendEntries from %d, at term %d", args.LeaderId, args.Term)
	rf.mu.Lock()
	defer rf.mu.Unlock()
	rf.printLog()
	rf.resetElectionTimeout()

	if args.Term > rf.currentTerm { // behind leader, become follower
		rf.makeFollowerN(args.Term)
	}

	// TODO: CHANGE
	success, term, index := rf.findConflict(args)
	if !success {
		rf.packageAppendEntryReply(reply, false, term, index)
		return
	}

	if args.Term < rf.currentTerm {
		rf.packageAppendEntryReply(reply, false, -1, -1)
		return
	}

	rf.debugLog("AppendEntries all checks passed from %d, at term %d. Applying %d entries", args.LeaderId, args.Term, len(args.Entries))

	//rf.debugLog("Current log length: %d, index: %d", len(rf.log), index)
	insertionPoint := args.PrevLogIndex + 1
	for i, entry := range args.Entries {
		if insertionPoint >= len(rf.log) {
			rf.log = append(rf.log, args.Entries[i:]...)
			rf.persist()
			break
		}

		// if append log is larger OR there is mismatch
		if rf.log[insertionPoint].Term != entry.Term {
			rf.log = append(rf.log[:insertionPoint], args.Entries[i:]...)
			rf.persist()
			break
		}
		insertionPoint++
	}

	// BOOKMARK
	rf.debugLog("args.LeaderCommit: %d, rf.commitIndex: %d", args.LeaderCommit, rf.commitIndex)
	if args.LeaderCommit > rf.commitIndex {
		rf.commitIndex = min(args.LeaderCommit, rf.getLastLogIndex())
		rf.debugLog("Leader commit index updated to %d", rf.commitIndex)
		rf.sendAppliedStates()
	}
	rf.packageAppendEntryReply(reply, true, -1, -1)

}

func (rf *Raft) sendAppliedStates() {
	rf.debugLog(fmt.Sprintf("Sending applied states 📝, lastApplied=%d, commitIndex=%d", rf.lastApplied, rf.commitIndex))
	//rf.mu.Lock()
	//defer rf.mu.Unlock()
	rf.debugLog(fmt.Sprintf("NUMBERO 2📝, lastApplied=%d, commitIndex=%d", rf.lastApplied, rf.commitIndex))
	// TODO: may be wrong

	for rf.lastApplied < rf.commitIndex {
		rf.lastApplied++
		rf.applyCh <- ApplyMsg{
			CommandValid: true,
			Command:      rf.log[rf.lastApplied].Command,
			CommandIndex: rf.lastApplied,
		}
		rf.debugLog("📝 applied log with rf.lastApplied=%d", rf.lastApplied)
		//rf.lastApplied++ // this here, may need to go at start (more likely end)

	}
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
	// Your code here (3B).
	rf.mu.Lock()
	defer rf.mu.Unlock()
	term := rf.currentTerm
	isLeader := (rf.state == LEADER)
	if isLeader {
		rf.log = append(rf.log, LogEntry{
			Term:    term,
			Command: command,
		})
		index = rf.getLastLogIndex()
		rf.persist()
		rf.debugLog("✅ Log appended at index %d", index)
	}

	return index, term, isLeader
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
	rf.persist()
	//rf.totalVotes = 1
	rf.mu.Unlock()
	go rf.StartElection()
}

func (rf *Raft) makeFollowerN(term int) {
	rf.resetElectionTimeout()
	rf.state = FOLLOWER
	rf.votedFor = -1
	rf.currentTerm = term
	rf.persist()
}

func (rf *Raft) makeLeader() {
	rf.resetElectionTimeout()
	if rf.state != CANDIDATE {
		return
	}

	rf.state = LEADER
	rf.nextIndex = make([]int, len(rf.peers))
	rf.matchIndex = make([]int, len(rf.peers))
	rf.debugLog(fmt.Sprintf("is now the leader"))

	for i := range len(rf.nextIndex) {
		rf.nextIndex[i] = rf.getLastLogIndex() + 1
		rf.matchIndex[i] = 0
	}
	go rf.leaderAppendEntries()
}

// runs once, sends an AppendEntries RPC to all peers
func (rf *Raft) leaderAppendEntries() {
	rf.debugLog(fmt.Sprintf("LeaderAppendEntries for term %d", rf.currentTerm))
	// should only run during a leader
	rf.mu.Lock()
	if rf.state != LEADER {
		rf.mu.Unlock()
		return
	}

	//println("Node ", rf.me, " is sending heartbeats for term ", rf.currentTerm)
	rf.debugLog(fmt.Sprintf("Sending heartbeats❤️❤️ to %d followers. Node killed is currently %t", len(rf.peers)-1, rf.killed()))

	rf.printLog()
	for peer := range rf.peers {
		if peer == rf.me {
			continue
		}
		args := AppendEntriesArgs{
			Term:         rf.currentTerm,
			LeaderId:     rf.me,
			PrevLogIndex: rf.getPrevLogIndex(peer),
			PrevLogTerm:  rf.getPrevLogTerm(peer),
			Entries:      append(make([]LogEntry, 0), rf.log[rf.nextIndex[peer]:]...),
			LeaderCommit: rf.commitIndex,
		}
		go func(peer int, entriesArgs AppendEntriesArgs) {
			rf.debugLog("Sending AppendEntries to %d", peer)
			var reply AppendEntriesReply
			res := rf.sendAppendEntries(peer, &entriesArgs, &reply)
			if res {
				rf.mu.Lock()
				defer rf.mu.Unlock()
				defer rf.printIndexes()
				if rf.state != LEADER || rf.currentTerm != args.Term {
					return
				}

				if reply.Term > rf.currentTerm {
					rf.debugLog(fmt.Sprintf("Making %d a follower due to lagged term", rf.me))
					rf.makeFollowerN(reply.Term)
					return
				}

				if reply.Success {
					rf.matchIndex[peer] = args.PrevLogIndex + len(args.Entries)
					rf.nextIndex[peer] = rf.matchIndex[peer] + 1
					rf.updateCommitIndexes(peer)
					return
				} else { // decrement nextIndex and retry.
					lastIndex := reply.ConflictingIndex

					if reply.ConflictingTerm != -1 {
						for i := 0; i < len(rf.log); i++ {
							if rf.log[i].Term == reply.ConflictingTerm {
								for i < len(rf.log) && rf.log[i].Term == reply.ConflictingTerm {
									i++
								}
								lastIndex = i
								break
							}
						}
					}

					rf.nextIndex[peer] = lastIndex
				}

			}
		}(peer, args)
	}
	rf.debugLog(fmt.Sprintf("LeaderAppendEntries for term %d done", rf.currentTerm))
	rf.mu.Unlock()
}
func (rf *Raft) updateCommitIndexes(peer int) {
	rf.debugLog("COMMIT INDEXES UPDATED for peer %d", peer)
	rf.matchIndex[rf.me] = len(rf.log) - 1

	for N := len(rf.log) - 1; N > rf.commitIndex; N-- {
		count := 0
		for _, matchIdx := range rf.matchIndex {
			if matchIdx >= N {
				count++
			}
		}

		if count > len(rf.matchIndex)/2 && rf.log[N].Term == rf.currentTerm {
			rf.commitIndex = N
			rf.sendAppliedStates()
			break
		}
	}
}

// The ticker go routine starts a new election if this peer hasn't received
// heartsbeats recently.
func (rf *Raft) ticker() {
	for rf.killed() == false {
		delayTime := time.Duration(rand.Intn(int(MaxElectionTimeout-MinElectionTimeout)) + int(MinElectionTimeout))
		time.Sleep(delayTime)

		rf.mu.Lock()
		state := rf.state
		shouldStart := rf.ShouldStartElection()

		rf.mu.Unlock()

		if state == FOLLOWER || state == CANDIDATE {
			if shouldStart {
				rf.debugLog(fmt.Sprintf("Election timeout, starting election"))
				rf.makeCandidate()
			} else {
				rf.debugLog("Nothing to do until next round")
			}
		}
	}
}

func (rf *Raft) leaderTicker() {
	for rf.killed() == false {
		rf.mu.Lock()
		state := rf.state
		rf.mu.Unlock()
		if state != LEADER {
			continue
		}
		rf.leaderAppendEntries()
		time.Sleep(HeartbeatInterval)
	}
}

// ============ LIFETIME MANAGEMENT ============

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
	rf.debugLog("❌ Killing node")
	atomic.StoreInt32(&rf.dead, 1)
	// Your code here, if desired.
}

func (rf *Raft) killed() bool {
	z := atomic.LoadInt32(&rf.dead)
	return z == 1
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

	rf.log = make([]LogEntry, 1)
	rf.applyCh = applyCh
	//rf.resetElectionTimeout()
	// initialize from state persisted before a crash
	rf.readPersist(persister.ReadRaftState())

	//println("Starting server ", rf.me, " with term ", rf.currentTerm)

	// start ticker goroutine to start elections
	go rf.ticker()
	go rf.leaderTicker()
	//go rf.mu.MonitorLock()

	return rf
}
