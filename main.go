package main

import (
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"log"
	"math/rand"
	"net/http"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"

	petname "github.com/dustinkirkland/golang-petname"
	"github.com/gorilla/websocket"
	//"github.com/kennygrant/sanitize"
)

/*
GAME CREATION
OBSERVER WATCHEAS LEADERBOARD
ALL PLAYERS CHOOSES TWO NUMBER BETWEEN 1 and 10
MINIMUM TWO PLAYERS MUST ENTER FOR GAME TO START. - MORE CAN JOIN
EACH GAME HAS 30 ROUNDS

EACH ROUND, THE SERVER WILL GENERATE A RANDOM NUMBER BETWEEN 1 and 10

User flow
1. Add name  - start session with userID
2. Pick Numbers -
3. Share with friends link

Game flow
1. Create game -
	b. Start routine for Game persistence

2. Count number of players
3. Start count-down to start once number of players surpass one
4. Start Game
5.

*/
//Message - Type structure for all messages going out of the system
type Message struct {
	Channel string `json:"channel"`
	Body    string `json:"body"`
}

//Wrap - Will return a json formated message
func (m *Message) Wrap(channel, body string) string {
	msg := new(Message)
	msg.Channel = channel //"GenMQ"
	msg.Body = body       //"Game " + g.Name + " created, will start when at least " + strconv.Itoa(MinimumGameEntry) + " players join...accepting entries"
	jsonMessage, err := json.Marshal(msg)
	if err != nil {
		fmt.Println(err.Error())
	}
	return string(jsonMessage)
}

//MessageQ - A simple message queue system that will act as a single source of message broadcast systemwide
type MessageQ struct {
	slice []string
}

//MQ - Global accessible message queue - Receives general messages for broadcast
var MQ *MessageQ = new(MessageQ)

//ScoreMQ - Receives only live score messages for broadcast
var ScoreMQ *MessageQ = new(MessageQ)

//LeadBoardMQ - Receive only leadboard messages for broadcast
var LeadBoardMQ *MessageQ = new(MessageQ)

var clients = make(map[*websocket.Conn]bool)    // connected clients
var clientsConn = make(map[*websocket.Conn]int) //connection type

//var broadcast = make(chan Message)           // broadcast channel

//Mutlock - primarily for write protection
var Mutlock sync.Mutex

//enQ - Add the string provided to the back of the queue
func (m *MessageQ) enQ(msg string) {
	// TODO: add msg to the queue -  DONE
	m.slice = append(m.slice, msg)
}

/*
func handleMessages() {
	for {
		// Grab the next message from the broadcast channel
		msg := <-broadcast
		// Send it out to every client that is currently connected
		for client := range clients {
			err := client.WriteJSON(msg)
			if err != nil {
				log.Printf("error: %v", err)
				client.Close()
				delete(clients, client)
			}
		}
	}
}*/

//deQ - Returns the first item in the queue and removes that item from the queue
//returns and error if there isnt one
func (m *MessageQ) deQ() (string, error) {
	if len(m.slice) < 1 {
		return "", errors.New("No message in queue")
	}
	fmsg := m.slice[0]
	m.slice = m.slice[1:len(m.slice)]
	return fmsg, nil
}

func (m *MessageQ) String() string {
	return fmt.Sprint(m.slice)
}

//Broadcast - sends all messages in message queue to subscribed channels - push
func (m *MessageQ) Broadcast() string {

	//For now just print to console screen
	// TODO: Send to websocket
	message, err := m.deQ()
	if err != nil {
		//fmt.Printf(err.Error())
	}
	if message != "" {
		//fmt.Println(message)
		return message
	}
	return ""
}

//broadCaster - Broadcasts messages to the MQ the general message queue channel
func broadCaster() {
	for {
		MQ.Broadcast()
		Delay(100, "ms")
	}
}

//scoreMQbroadCaster - Broadcasts messages exclusively to the scoreMQ channel
func scoreMQbroadCaster() {
	for {
		ScoreMQ.Broadcast()
		Delay(100, "ms")
	}
}

//LeadboardMQbroadCaster - Broadcasts messages exclusively to the scoreMQ channel
func leadBoardMQbroadCaster() {
	for {
		LeadBoardMQ.Broadcast()
		Delay(1, "s")
	}
}

//MinimumGameEntry - Minimum number of players needed for a game to start.
var MinimumGameEntry = 2

//Player - Holds structure for player-users
type Player struct {
	ID   string `json:"id"`
	Name string `json:"name"`
}

//Players  - holds a list of all players
var Players []Player

//FakePlayers - Fake players for test purposes
var FakePlayers bool

//FakePlayerList - Stores a list of fake players for when it needs to regenerate fake plays
var FakePlayerList = make(map[string]string)

//AddPlayer - Adds/Registers a new player to the game.
func AddPlayer(Name string) string {
	msg := new(Message)
	P := new(Player)
	P.ID = randomString(8)
	P.Name = Name
	Players = append(Players, *P)
	MQ.enQ(msg.Wrap("GenMQ", "New player "+Name+" added to pool."))
	return P.ID
}

//Play - Holds structure of users' game play
//This serves as the queue to be loaded into the Game when the current one finishes and/or a new one starts.
type Play struct {
	PlayerID   string `json:"playerid"`
	PlayerName string `json:"playername"`
	Entries    [3]int `json:"entries"`
}

//Plays - Holds a list of game plays by users
var Plays []Play

//AddPlay - Adds a User's play to the play queue
func AddPlay(PlayerID, PlayerName string, entries [3]int) {
	msg := new(Message)
	var plaid int
	for _, v := range Plays {
		if v.PlayerID == PlayerID {
			plaid++
		}
	}
	if plaid < 1 {
		var P = new(Play)
		P.PlayerID = PlayerID
		P.PlayerName = PlayerName
		P.Entries = entries
		//add to queue
		Plays = append(Plays, *P)
		n1 := strconv.Itoa(P.Entries[0])
		n2 := strconv.Itoa(P.Entries[1])
		//
		MQ.enQ(msg.Wrap("GenMQ", "New play entry made by <strong>=> "+PlayerName+"</strong>("+n1+" and "+n2+")"))
	}
	//fmt.Println(Plays)
}

//PurgePlays - will remove all the plays currently in queue
func PurgePlays() {
	Plays = Plays[:0]
}

/*::::::::::::::::::::::::::::::::::::::::::::::::::::::::::::::::::::::::::::::::::::::::
- Game methods and functions
::::::::::::::::::::::::::::::::::::::::::::::::::::::::::::::::::::::::::::::::::::::::::*/

//Game - Holds structure dscriptions for game type
type Game struct {
	ID          string    `json:"id"`
	Name        string    `json:"name"`
	Starttime   time.Time `json:"starttime"`
	Endtime     time.Time `json:"endtime"`
	RoundScores [30]int   `json:"roundscores"`
	Plays       []Play    `json:"plays"`
	Status      string    `json:"status"`
}

//Games - Hold a list of games in Memory
var Games []Game

//GlistDynamic - Hold a changing list of games for status updates. #hack
var GlistDynamic = make(map[string]string)

//UpdateGlistDynamic - Update The list of Games for status report updates
func UpdateGlistDynamic(GName, Gstatus string) {
	//GlistDynamic = GlistDynamic[:0]
	for i := range GlistDynamic {
		if i == GName {
			GlistDynamic[i] = Gstatus
		}
	}

}

//SetGlistDynamic - Inpute The list of Games for status report updates
func SetGlistDynamic(GName, Gstatus string) {
	//GlistDynamic = GlistDynamic[:0]
	GlistDynamic[GName] = Gstatus
}

//Create - Greates a new game
//@param
//@return Game
func Create() *Game {
	var G *Game = new(Game)
	//var ScoreMQ *MessageQ = new(MessageQ)
	//set Game properties
	G.SetID()
	//send alerts to message queue
	G.SetPetName()
	G.SetStartScores()
	G.SetStatus("Waiting")
	SetGlistDynamic(G.Name, G.Status)
	//makes sure there's no conflict in unique parameters like ID
	return G
}

//SetID - Sets a unique 8 character random string as ID
func (g *Game) SetID() {
	id := randomString(8)
	for isDuplicateGID(id) == true {
		id = randomString(8)
	}
	g.ID = id
}

//SetPetName - Sets two dictionary words as game pet name
func (g *Game) SetPetName() {
	rand.Seed(time.Now().UnixNano())
	g.Name = petname.Generate(2, "-")
}

//SetStartScores - set all game round scores to zero for starters
func (g *Game) SetStartScores() {
	for i := 0; i < len(g.RoundScores); i++ {
		g.RoundScores[i] = 0
	}
}

//isDuplicateGID - returns true if a duplicate is found of the given Game ID
func isDuplicateGID(id string) bool {
	for _, g := range Games {
		if g.ID == id {
			return true
		}
	}
	return false
}

//SetStatus -
func (g *Game) SetStatus(status string) {
	g.Status = status
}

//SetPlays -
//1. Count Current Queued plays.
//2. Starts the count down timer when the total plays gets to or surpas the minimum i.e 2 plays.
//3. Listens for additional plays while counting down and ...
//4. Loads all queued/submitted plays into the current game, locks the game and ...
//5. Starts the game when timer times out at 10 seconds.
func (g *Game) SetPlays(playedChan chan int, wg *sync.WaitGroup) {
	//get queued plays
	//fmt.Printf("Game name: %v Game status: %v \n\n", g.Name, g.Status)
	var body string
	var channel string

	msg := new(Message)

	//channel = "GenMQ"
	//body = "Game " + g.Name + " created, will start when at least " + strconv.Itoa(MinimumGameEntry) + " players join...accepting entries"
	//MQ.enQ(msg.Wrap(channel, body))

	//MQ.enQ("GenMQ: Game " + g.Name + " created, will start when at least " + strconv.Itoa(MinimumGameEntry) + " players join...accepting entries")
	//MQ.enQ("GenMQ: Checking for minimum of " + strconv.Itoa(MinimumGameEntry) + " players before start .	.	.	.	.	.	.")
	channel = "GenMQ"
	body = "Checking for minimum of " + strconv.Itoa(MinimumGameEntry) + " players before start .	.	.	.	.	.	."
	MQ.enQ(msg.Wrap(channel, body))
	//Display list of players and their plays

	for g.Status == "Waiting" {

		if len(Plays) >= MinimumGameEntry {
			//minimum achieved
			g.Status = "Live"
			g.Starttime = time.Now()
			g.SetStatus("Live")

			//fmt.Printf("Plays slice : %v \n", Plays)
			MQ.enQ(msg.Wrap("GenMQ", "All pending play entries have been accepted entries are now closed for this game"))

			var xPlays []Play
			for _, v := range Plays {
				xPlays = append(xPlays, v)
			}
			g.Plays = xPlays
			MQ.enQ(msg.Wrap("AnnouncmentMQ", "Game "+g.Name+" is starting in 10 seconds"))

			go func() {
				cd, err := SendCountDown(10, "s")
				RunError(err)
				fmt.Printf("Sent all countdown to clients. %v seconds left \n", cd)

			}()

			Delay(10, "s")
			g.Start(playedChan, wg)

		} else {

			currentEntries := "Current Game entries<br>\n\n"
			for _, v := range Plays {
				PlayerName := v.PlayerName
				PlayerEntry := "[" + strconv.Itoa(v.Entries[0]) + "," + strconv.Itoa(v.Entries[1]) + "," + strconv.Itoa(v.Entries[2]) + "]"
				currentEntries = currentEntries + "<br> Player's Name : " + PlayerName + ":" + PlayerEntry + "\n"
				//currentEntries = currentEntries + "\n"
			}
			//g.Status = "Waiting" witing status already set. No need here
			MQ.enQ(msg.Wrap("GenMQ", "Waiting for more players to join "+g.Name+" =>  I'll try agian in 15 seconds"))
			MQ.enQ(msg.Wrap("GenMQ", currentEntries))
			Delay(15, "s")
		}
	}
}

//Start - Starts a game and changes the status and start times parameters of a given game that has not previously ended
//Calls Play Game and maintains a one second delay.
//@Param GiD - string Game ID
func (g *Game) Start(playedChan chan int, wg *sync.WaitGroup) {
	defer wg.Done()
	msg := new(Message)
	//check previous game status to make sure it's not an old ended game.
	//Plays = Plays[len(Plays):]
	PurgePlays()
	//PurgeGlistDynamic()
	//Purge game list
	//repopulate it.
	//CurrentgametitlefeedMQ
	MQ.enQ(msg.Wrap("CurrentgametitlefeedMQ", "Currently playing: "+g.Name))
	//fmt.Printf("Plays slice : %v \n", Plays)
	MQ.enQ(msg.Wrap("GenMQ", "All pending play entries have been accepted"))
	g.Starttime = time.Now()
	roundsPlayed := 0
	for i := 0; i < len(g.RoundScores); i++ {
		if g.Status == "Live" {
			UpdateGlistDynamic(g.Name, g.Status)
			//g.SetStatus("Live")
			MQ.enQ(msg.Wrap("GenMQ", "Starting Round "+strconv.Itoa(i+1)+" of game : "+g.Name+" &raquo;["+g.Status+"]\n"))
			ScoreMQ.enQ(msg.Wrap("LiveScoreMQ", "Starting Round "+strconv.Itoa(i+1)+"\n"))
			MQ.enQ(msg.Wrap("GenMQ", "Starting Round "+strconv.Itoa(i+1)+"\n"))
			//MQ.enQ(msg.Wrap("GenMQ", "Round Random Number before ==> "+strconv.Itoa(g.RoundScores[i])))
			//fmt.Printf("Round Random Number before : %v \n", g.RoundScores[i])
			//1. Check for zero entry
			//2. Check for winner number 21 score
			if g.RoundScores[i] == 0 {
				g.Play(i)
			}
			//fmt.Printf("Round Random Number after : %v \n", g.RoundScores[i])
			MQ.enQ(msg.Wrap("GenMQ", "Round Game number <strong> => "+strconv.Itoa(g.RoundScores[i])+"</strong>"))
			//fmt.Println(g.RoundScores) //checking
			Delay(1, "s")
			roundsPlayed++
		}
	}
	playedChan <- roundsPlayed //total rounds played
}

//Gamewide lowest live score
//var LowLiveScore int

//Score - Calculate scores for all entries in the game for that particular play
//@param p int play number
func (g *Game) Score(p int) bool {
	msg := new(Message)
	// TODO: Calculate and set score all entries given the random number generated by the game N
	score := g.RoundScores[p]
	//scoring...
	LiveScore := "Low#&nbsp;&nbsp;&nbsp;&nbsp;&nbsp;&nbsp;High# &nbsp;&nbsp;&nbsp;&nbsp;&nbsp;&nbsp;&nbsp;Score &nbsp;&nbsp;&nbsp;&nbsp;&nbsp;&nbsp;&nbsp;&nbsp;Player "
	var TopLiveScore int
	var LowLiveScore int
	//exact match of upper or lower number = +5 points
	//insideBounds of chosen number +5 - (upperBound - lowerBound)
	for i, v := range g.Plays {

		if g.Status == "Live" {
			minE := v.Entries[0]
			maxE := v.Entries[1]
			totalScore := v.Entries[2]

			//MQ.enQ("Plays before calc " + strconv.Itoa(i) + " : " + strconv.Itoa(v.Entries) + "  random score => " + strconv.Itoa(score) + "\n")

			//Match
			if score == maxE || score == minE {
				totalScore = v.Entries[2] + 5
			}
			//Inside-Bounds
			if score > minE && score < maxE {
				totalScore = v.Entries[2] + (5 - (maxE - minE))
			}
			//Out of bounds
			if score < minE || score > maxE {
				totalScore = v.Entries[2] - 1
			}
			//v.Entries[2] = totalScore

			g.Plays[i].Entries[2] = totalScore

			//print to score board - send JSON for live demo

			//LiveScore := v.PlayerName + " Lower => " + strconv.Itoa(v.Entries[0]) + " : " + strconv.Itoa(v.Entries[1]) + " <= Higher" + " --> Current Score : " + strconv.Itoa(v.Entries[2]) + "\n"
			//ScoreMQ.enQ(msg.Wrap("LiveScoreMQ", LiveScore))

			LiveScore = LiveScore + "<br>Low: " + strconv.Itoa(v.Entries[0]) + "&nbsp;&nbsp;&nbsp;&nbsp;High: " + strconv.Itoa(v.Entries[1]) + "&nbsp;&nbsp;&nbsp;&nbsp;&nbsp;&nbsp;Score: " + strconv.Itoa(totalScore) + "&nbsp;&nbsp;&nbsp;&nbsp;&nbsp;" + v.PlayerName
			//LiveScore = LiveScore + "<br >" + v.PlayerName + "&nbsp;&nbsp;&nbsp;&nbsp;Low: " + strconv.Itoa(v.Entries[0]) + "&nbsp;&nbsp;&nbsp;&nbsp;High: " + strconv.Itoa(v.Entries[1]) + "&nbsp;&nbsp;&nbsp;&nbsp;&nbsp;&nbsp;Score: " + strconv.Itoa(v.Entries[2])

			if totalScore > TopLiveScore {
				TopLiveScore = totalScore
				TopLiveScorer := "TOP SCORE&raquo; " + g.Plays[i].PlayerName + ":&nbsp;<strong>" + strconv.Itoa(g.Plays[i].Entries[2]) + "</strong>"
				MQ.enQ(msg.Wrap("LivetopscorerMQ", TopLiveScorer))
			}

			if totalScore < LowLiveScore {
				LowLiveScore = totalScore
				LowLiveScorer := "LOW SCORE&raquo; " + g.Plays[i].PlayerName + ":&nbsp;<strong>" + strconv.Itoa(g.Plays[i].Entries[2]) + "</strong>"
				MQ.enQ(msg.Wrap("LivelowscorerMQ", LowLiveScorer))
			}

			if totalScore == 21 {

				if g.Stop() == true {
					_ = g.Tally(g.ID) //Generates LeaderBoard
				}
				braodCastLiveScores(LiveScore)
				//Redisplay scores
				//braodCastLiveScores()
				//ScoreMQ.enQ(msg.Wrap("LiveScoreMQ", "21 Point Winner!!!"))
				//MQ.enQ(msg.Wrap("AnnouncmentMQ", "21 Point Winner!!!"))
				return false
			}
		}

		//final play and end the game
		if p == len(g.RoundScores)-1 {
			MQ.enQ(msg.Wrap("GenMQ", "Final play, ending game..."))
			if g.Stop() == true {
				_ = g.Tally(g.ID) //Generates LeaderBoard
			}
			//Redisplay scores
			//braodCastLiveScores()
			braodCastLiveScores(LiveScore)
			return false
		}
		//fmt.Println(g.Plays[i].Entries)
		//c.Entries[2] = totalScore
		//fmt.Printf("Plays after calc %v : %v  random score => %v : ---TotalScore %v\n", i, g.Plays[i].Entries, score, totalScore)
		//Redisplay scores
		//braodCastLiveScores()
	}
	braodCastLiveScores(LiveScore)

	return true

}

//Stop - Stops a game and changes the status and end time parameters of a given game that is running
//@Param GiD - string Game ID
//@return boolean
func (g *Game) Stop() bool {
	//check previous game status to make sure it's not an old ended game.
	//set the game endtime property
	//set the game status
	g.Status = "Ended"
	g.Endtime = time.Now()
	return true
}

//Play - Plays a game. Calls the tally function after every play to determine
//if a winner has been found, also to decide if game should continue
func (g *Game) Play(i int) {
	msg := new(Message)
	//Random integer number
	if g.Status == "Live" {
		g.RoundScores[i] = int(randInt(1, 10, -1))
		if g.Score(i) == false {
			//g.Status = "Ended"
			MQ.enQ(msg.Wrap("GenMQ", "Game "+g.Name+" has ended"))
			return
		}
	}
	return
}

//LeadBoard - stores wins and tracks game stats accross games
type LeadBoard struct {
	GameID     string    `json:"gameid"`
	GameName   string    `json:"gamename"`
	GameStatus string    `json:"gamestatus"`
	Starttime  time.Time `json:"starttime"`
	Endtime    time.Time `json:"endtime"`
	TopScorer  []Play
	LowScorer  []Play
	Winner     []Play
}

//LeadB - array of game records
var LeadB []LeadBoard

//Winners hold the list of top scorers
var Winners []Play

//Tally - Calculate the totals of the game to see who the winner is not.
//@param GiD - string Game ID
//@return string - status report which tells the game to either end when a winner is found or continue to next play
func (g *Game) Tally(GiD string) LeadBoard {
	msg := new(Message)
	//LeadBoard format - UserID - UserName - UserLowerPlay - UserUperPlay -  GameID - GameName -

	//fmt.Println(GamePlayers)
	//jgx, _ := json.Marshal(g)
	//fmt.Println(string(jgx))
	//KEY DATA to Extract
	//GAME ID
	//GAME NAME
	//STATUS
	//TIME STARTED
	//TIME ENDED
	//==> PLays<==// [{CMAKHCJC flea [3 9 3]} {CMAKHCJC flea [7 8 3]} {XRKISJDD dogfish [3 8 21]} {ZFCKXWLU baboon [3 9 4]} {LWBWCNQB guinea [3 8 22]} {LXEAIRJI badger [7 9 4]}]
	//TopScorer
	//Low Scorer
	//Winner (by score, by jackpot, by name in case of a tie)
	//
	var TopScorer = make([]Play, 1)
	var LowScorer = make([]Play, 1)
	var WinScorer = make([]Play, 1)
	//var JackScorer = make([]Play, 1)
	//var TopTieScorer = make([]Play, len(g.Plays))
	var TopTieScorer []Play //may cause index range errors check properly
	var LowTieScorer = make([]Play, len(g.Plays))
	//var JackTieScorer = make([]Play, len(g.Plays))
	var JackTieScorer []Play //= make([]Play, 0)

	glb := new(LeadBoard)
	glb.GameID = g.ID
	glb.GameName = g.Name
	glb.GameStatus = g.Status
	glb.Starttime = g.Starttime
	glb.Endtime = g.Endtime
	//glb.TopScorer = TopScorer
	//glb.LowScorer = LowScorer
	//glb.Winner = WinScorer
	//GameTitle := g.Name
	//	var lastTopPlay int
	var currentTopScore int
	var currentLowScore = g.Plays[0].Entries[2]

	for i, v := range g.Plays {
		if v.Entries[2] == 21 {
			//JackScorer[0] = v //no need
			JackTieScorer = append(JackTieScorer, v)
		}

		if i == 0 {
			//set first current top score and low scores
			currentTopScore = v.Entries[2]
			currentLowScore = v.Entries[2]
		}

		if v.Entries[2] > currentTopScore {
			TopScorer[0] = v
			currentTopScore = v.Entries[2]
			TopTieScorer = TopTieScorer[:0] //Clean or empty up tie
			//TopTieScorer = append(TopTieScorer, v)
		}

		if v.Entries[2] < currentLowScore {
			LowScorer[0] = v
			currentLowScore = v.Entries[2]
			LowTieScorer = LowTieScorer[:0] //Clean or empty up tie
		}

		//A winner Tie
		if v.Entries[2] == currentTopScore {
			TopTieScorer = append(TopTieScorer, v)
		}

		//A looser Tie
		if v.Entries[2] == currentLowScore {
			LowTieScorer = append(LowTieScorer, v)
		}
	}

	if len(JackTieScorer) < 1 {
		//no jack pot winner use top scorer for wins
		if len(TopTieScorer) > 1 {
			//there's a tie
			//fmt.Println(TopTieScorer)
			//sort by players high input number
			sort.SliceStable(TopTieScorer, func(i, j int) bool { return TopTieScorer[i].Entries[1] > TopTieScorer[j].Entries[1] })
			if TopTieScorer[0].Entries[1] == TopTieScorer[1].Entries[1] {
				//MQ.enQ(msg.Wrap("AnnouncmentMQ", "Entry compare6: ["+strconv.Itoa(TopTieScorer[0].Entries[1])+"] &nbsp; ["+strconv.Itoa(TopTieScorer[1].Entries[1])+"]"))
				//MQ.enQ(msg.Wrap("AnnouncmentMQ", "Entry compare6low: ["+strconv.Itoa(TopTieScorer[0].Entries[0])+"] &nbsp; ["+strconv.Itoa(TopTieScorer[1].Entries[0])+"]"))

				//there's a tie sort by players low input number
				sort.SliceStable(TopTieScorer, func(i, j int) bool { return TopTieScorer[i].Entries[0] > TopTieScorer[j].Entries[0] })
				if TopTieScorer[0].Entries[0] == TopTieScorer[1].Entries[0] {
					//there's still a tie
					//sort by name score is the same
					sort.SliceStable(TopTieScorer, func(i, j int) bool { return TopTieScorer[i].PlayerName < TopTieScorer[j].PlayerName })
					WinScorer[0] = TopTieScorer[0]
					TopScorer[0] = TopTieScorer[0]
					//MQ.enQ(msg.Wrap("AnnouncmentMQ", "Case 5 name winner Winner!!"))
					//MQ.enQ(msg.Wrap("AnnouncmentMQ", "Entry compare5: ["+strconv.Itoa(TopTieScorer[0].Entries[1])+"] &nbsp; ["+strconv.Itoa(TopTieScorer[1].Entries[1])+"]"))
					//MQ.enQ(msg.Wrap("AnnouncmentMQ", "Entry compare5 low: ["+strconv.Itoa(TopTieScorer[0].Entries[0])+"] &nbsp; ["+strconv.Itoa(TopTieScorer[1].Entries[0])+"]"))

				} else {
					WinScorer[0] = TopTieScorer[0]
					//MQ.enQ(msg.Wrap("AnnouncmentMQ", "Case 4 highest lowest chosen number Winner!!"))
					//MQ.enQ(msg.Wrap("AnnouncmentMQ", "Entry compare 4: ["+strconv.Itoa(TopTieScorer[0].Entries[1])+"] &nbsp; ["+strconv.Itoa(TopTieScorer[1].Entries[1])+"]"))

				}

			} else {

				WinScorer[0] = TopTieScorer[0]
				//MQ.enQ(msg.Wrap("AnnouncmentMQ", "Case 3 Highest chosen number Winner!!"))
				//MQ.enQ(msg.Wrap("AnnouncmentMQ", "Entry compare 3: ["+strconv.Itoa(TopTieScorer[0].Entries[1])+"] &nbsp; ["+strconv.Itoa(TopTieScorer[1].Entries[1])+"]"))
			}
			//
			MQ.enQ(msg.Wrap("AnnouncmentMQ", "High Score Winner!!"))
		} else {
			//MQ.enQ(msg.Wrap("AnnouncmentMQ", "Case 2 Simple highest scorer Winner!!"))
			WinScorer[0] = TopScorer[0]
		}

	} else {

		//there's a jackpot winner
		//if len(JackTieScorer) > 1 { //using greater than cause we're putting in anyway as long as they score 21 so just check for count above 1
		//choose from the top name of the jackpot tie
		sort.SliceStable(JackTieScorer, func(i, j int) bool { return JackTieScorer[i].PlayerName < JackTieScorer[j].PlayerName })
		WinScorer[0] = JackTieScorer[0]
		MQ.enQ(msg.Wrap("AnnouncmentMQ", "Jack pot Winner!!"))
		//} else {
		//or just the jackpot scorer
		//	WinScorer[0] = JackScorer[0]
		//}*/
		MQ.enQ(msg.Wrap("AnnouncmentMQ", "21 Point Winner!!!"))
	}

	glb.TopScorer = TopScorer
	glb.LowScorer = LowScorer
	glb.Winner = WinScorer

	//fmt.Println(GamePlayers)
	//jglb, _ := json.Marshal(glb)
	//fmt.Println(string(jglb))

	//gj, err := json.Marshal(glb)
	//if err != nil {
	//	fmt.Println(err.Error())
	//}
	//MQ.enQ(msg.Wrap("GenMQ", "Game TALLY ====> \n\n"+string(gj)))
	//end print out the game data

	//MQ.enQ(msg.Wrap("GenMQ", "Winner is : "+glb.Winner[0].PlayerName))
	MQ.enQ(msg.Wrap("AnnouncmentMQ", "Winner is  <br>"+glb.Winner[0].PlayerName+"<br> Winning score : "+strconv.Itoa(glb.Winner[0].Entries[2])))
	Winners = append(Winners, glb.Winner[0])
	//top scorer table
	tops, err := TallyTopScorers()
	RunError(err)
	fmt.Printf("Published  %v new topscorer", tops)
	//bradCastleadBoard()
	//bradCastGameList()
	return *glb
}

//TallyTopScorers - publishes a list of top scorer to the connected clients.
func TallyTopScorers() (int, error) {
	msg := new(Message)
	Mutlock.Lock()
	Winnersx := Winners
	Mutlock.Unlock()
	var topscorer string
	winnerCount := len(Winnersx)

	var TName string
	var TScore int

	if winnerCount > 0 {
		sort.SliceStable(Winnersx, func(i, j int) bool { return Winnersx[i].Entries[2] > Winnersx[j].Entries[2] })
		if len(Winnersx) > 1 && Winnersx[0].Entries[2] == Winnersx[1].Entries[2] {
			//sort by name
			Winnersx2 := Winnersx[:2]
			sort.SliceStable(Winnersx2, func(i, j int) bool { return Winnersx2[i].Entries[2] < Winnersx2[j].Entries[2] })
			//TName = Winnersx2[len(Winnersx2)-1].PlayerName
			//TScore = Winnersx2[len(Winnersx2)-1].Entries[2]
			TName = Winnersx2[0].PlayerName
			TScore = Winnersx2[0].Entries[2]
		} else {
			//sort.SliceStable(Winnersx, func(i, j int) bool { return Winnersx[i].Entries[2] < Winnersx[j].Entries[2] })
			//TName = Winnersx[len(Winnersx)-1].PlayerName
			//TScore = Winnersx[len(Winnersx)-1].Entries[2]
			TName = Winnersx[0].PlayerName
			TScore = Winnersx[0].Entries[2]
		}

		topscorer = TName + "<br><strong>" + strconv.Itoa(TScore) + "</strong>"
		MQ.enQ(msg.Wrap("AlltimescorerMQ", topscorer))
		return 1, nil
	}
	return 0, errors.New("No winner to publish in list")
}

//bradCastleadBoard -
func bradCastleadBoard() {
	msg := new(Message)
	var GamesList string
	//for {
	//range Leadboard
	for _, v := range LeadB {

		w := v.Winner
		if v.GameStatus == "Ended" {
			GamesList += v.GameName + "&nbsp; [" + v.GameStatus + "] Winner: " + w[0].PlayerName + " Entries : " + strconv.Itoa(w[0].Entries[0]) + " & " + strconv.Itoa(w[0].Entries[1])
			MQ.enQ(msg.Wrap("LeadBoardFMQ", GamesList))
		} else {
			GamesList += v.GameName + "&nbsp; [" + v.GameStatus + "]"
			MQ.enQ(msg.Wrap("LeadBoardMQ", GamesList))
		}
	}
	Delay(5, "s")
	//	}
}

//bradCastleadBoard -
func broadCastGameList() {

	for {
		x := 1
		msg := new(Message)
		var GamesList string
		var LiveGamesList string

		//Make a fresh copy of games lists
		//range Leadboard
		Mutlock.Lock()
		for i, v := range GlistDynamic {
			//sort.SliceStable(JackTieScorer, func(i, j int) bool { return JackTieScorer[i].PlayerName < JackTieScorer[j].PlayerName })
			if v == "Waiting" {

				GamesList += "<div id=\"alltimebest\" class=\"regularbar lefttext\">" + "&nbsp;" + i + " &raquo status [" + v + "]</div>"
				//"<div id=\"alltimebest\" class=\"regularbar\">"+v.Name +"</div>"
				x++
			}

			if v == "Live" {
				LiveGamesList = "<div id=\"alltimebest\" class=\"eventbar lefttext\">" + "&nbsp;" + i + " &raquo status [" + v + "]</div>"
			}
		}
		Mutlock.Unlock()
		MQ.enQ(msg.Wrap("GameListMQ", LiveGamesList+GamesList))
		MQ.enQ(msg.Wrap("LiveGameMQ", LiveGamesList))
		Delay(2, "s")
	}

}

func braodCastLiveScores(LiveScore string) {
	msg := new(Message)
	MQ.enQ(msg.Wrap("LiveScoreMQ", LiveScore))
	//Delay(2, "s")
	/*var CGName string
	var MPlays []Play
	for {

		//chekc for live games
		for n, g := range GlistDynamic {
			if g == "Live" {
				CGName = n
			}
		}
		//set content from live game
		for _, g := range Games {
			if g.Name == CGName {
				MPlays = g.Plays
			}
		}
		//use content
		currentEntries := ""
		for _, v := range MPlays {
			PlayerName := v.PlayerName
			PlayerEntry := "[&nbsp" + strconv.Itoa(v.Entries[0]) + "&nbsp; - &nbsp;" + strconv.Itoa(v.Entries[1]) + "&nbsp;] \t\t&nbsp; <strong class=\"righttext\">Score: " + strconv.Itoa(v.Entries[2]) + "</strong>"

			currentEntries = currentEntries + "<br>" + PlayerName + "\t\t" + PlayerEntry + "\n"

			//currentEntries = currentEntries + "\n"
		}* /


	}*/

}

/*/SaveGames - Saves list of []Games to storage. Called by independent Go routine on intervals
//@return bool, error
func SaveGames() (bool, error) {

	return true, nil
}*/

//InitiateGame - Initiate games monitor them and keep track their progress.
func InitiateGame(gameCount int) int {
	msg := new(Message)

	var wg sync.WaitGroup

	for x := 1; x <= gameCount; x++ {
		G := Create()
		//store in array of games
		Games = append(Games, *G)
	}
	//
	//bradCastleadBoard()
	go broadCastGameList()
	//Games on Board
	//braodCastLiveScores()
	//publish entries
	//go braodCastLiveScores()
	// TODO: Remove or refine
	//pgob, _ := json.Marshal(Games)
	//fmt.Println(string(pgob))

	playedChan := make(chan int) //track plaid rounds

	pld := 0
	wg.Add(len(Games))
	for _, game := range Games {

		cG := game
		//fmt.Printf("Starting game : %v \n\n", cG.Name)
		//go func(wgc *sync.WaitGroup) {
		//MQ.enQ(msg.Wrap("AnnouncmentMQ", "Starting game "+cG.Name+" in 10 seconds"))
		createRandomPlays() // TODO : Remove from live submission
		Delay(10, "s")
		wg.Add(1)
		go cG.SetPlays(playedChan, &wg)
		//fmt.Printf("%v Rounds where played \n\n", <-playedChan)
		MQ.enQ(msg.Wrap("GenMQ", strconv.Itoa(<-playedChan)+" Rounds were played \n\n"))
		//Update list of games for status report
		UpdateGlistDynamic(cG.Name, cG.Status)
		//Delay(60, "s")

		//waiting thirty seconds before game start
		//Delay(delayTime, "s")
		pld++
		//fmt.Printf("Closing game : %v \n\n", cG.Name)
		MQ.enQ(msg.Wrap("GenMQ", "Closing game "+cG.Name))
		wg.Done()
	}
	//if all games have been played end the program
	wg.Wait()
	close(playedChan)
	return pld
}

//AddUser - Takes input from user and creates a profile for them
func AddUser(PlayerName string) string {
	playerID := AddPlayer(PlayerName)
	return playerID
}

//fakeUsersPlay - Generates fake users for testing purpose
func fakeUsersPlay(userCount int) {
	//add player
	for i := 0; i < userCount; i++ {
		PlayerName := petname.Generate(1, "")
		PlayerID := AddPlayer(PlayerName)
		FakePlayerList[PlayerName] = PlayerID
	}
	//create random plays for testing
	createRandomPlays() // TODO : Remove from live submission
}

//createRandomPlays - Generate random game plays inputs for already existing users
func createRandomPlays() {
	msg := new(Message)

	if FakePlayers == true {
		//Create new plays for each player....assume all will join new game
		for _, v := range Players {

			for fakename := range FakePlayerList {
				if fakename == v.Name {
					//add player
					//for i := 0; i <= userCount; i++ {
					entry1 := randInt(1, 10, -1)
					entry2 := randInt(1, 10, entry1)
					var totalScore int = 0
					// TODO: check for empty or invalid entries
					// TODO: sort entries for ease of use for threshold marker
					if entry1 > entry2 {
						entry1, entry2 = entry2, entry1
					}

					PlayerName := v.Name //petname.Generate(1, "")
					//fmt.Printf("Name %v \n", name)
					playerID := v.ID //AddPlayer(name)

					ent := [3]int{int(entry1), int(entry2), int(totalScore)}
					AddPlay(playerID, PlayerName, ent)

					MQ.enQ(msg.Wrap("GenMQ", "Total plays made so far: "+strconv.Itoa(len(Plays))))
					Delay(1, "s")
				}
			}
			//}
		}
	}
}

func serveFiles(w http.ResponseWriter, r *http.Request) {
	//fmt.Println(r.URL.Path)
	p := "." + r.URL.Path
	if p == "./" {
		p = "./index.html"
	}
	http.ServeFile(w, r, p)
}

/*
//CreatePlayer - Handles api end point request for creating robots
func CreatePlayer(w http.ResponseWriter, r *http.Request) {
	type RobReq struct {
		Rname string `json:"rname"`
		Rtype string `json:"rtype"`
	}

	var robo []RobReq
	jsonDcode := json.NewDecoder(r.Body)
	err := jsonDcode.Decode(&robo)
	if err != nil {
		panic(err)
	}
	for _, v := range robo {
		//CreatePlayer(sanitize.Name(v.Rname), sanitize.Name(v.Rtype))
	}
}*/

//UTILITY - Some Utility functions
//randomString -
func randomString(l int) string {
	//seed generator
	//rand.Seed(time.Now().UTC().UnixNano())
	bytes := make([]byte, l)
	for i := 0; i < l; i++ {
		bytes[i] = byte(randInt(65, 90, -1))
	}
	return string(bytes)
}

//ranInt - returns a random integer between given minimum and maximum inclusive except integer passed in omit
//@param min, max int
//@return int
func randInt(min, max, omit int) int {
	//seed generator
	rand.Seed(time.Now().UTC().UnixNano())

	rN := min + rand.Intn(max-(min-1))
	if omit > 0 {
		for rN == omit {
			rN = min + rand.Intn(max-(min-1))
		}
	}
	return rN
}

//Delay - is a play on time functions to make usage more easily repetetive and less character usage
//@param duration int - duration of the time delay
//@param timescale string - what time scale what the delay be in. use "s" => seconds and "ms" => Milliseconds
func Delay(duration int, timescale string) error {
	if timescale == "ms" {
		time.Sleep(time.Millisecond * time.Duration(duration))
		return nil
	}

	if timescale == "s" {
		time.Sleep(time.Second * time.Duration(duration))
		return nil
	}
	return nil
}

//SendCountDown -
func SendCountDown(duration int, timescale string) (int, error) {
	msg := new(Message)
	if timescale == "s" {
		for i := 0; i <= duration; i++ {
			MQ.enQ(msg.Wrap("CountdownMQ", strconv.Itoa(duration-i)))
			time.Sleep(time.Second * time.Duration(1))
		}
		MQ.enQ(msg.Wrap("CountdownMQ", "-"))
		return 1, nil
	}
	return 0, nil
}

/*
::::::::::::::::::::::::::::::::::::::::::::::::::::::::::::::::::::::::::::::
Start of main functions
::::::::::::::::::::::::::::::::::::::::::::::::::::::::::::::::::::::::::::::
*/
func init() {
	rand.Seed(time.Now().UTC().UnixNano())
}

func main() {

	//use wait groups to keep main on hold
	var wg sync.WaitGroup

	msg := new(Message)
	flag.Parse()

	args := flag.Args()

	if len(args) < 3 {
		fmt.Println("Please specify the following : a number_of_games port_to_run_web_interface minimum_player_number")
		fmt.Println("E.G: $ go run game.go 3 2020 2")
		//os.Exit(1)
		alt := []string{"3", "8080", "2"}
		args = alt
	}

	//end user addition and plays
	//wg.Add(1)
	gameCount, err := strconv.Atoi(args[0]) //0
	if err != nil {
		fmt.Println(err.Error())
	}

	webPort, err := strconv.Atoi(args[1]) //1
	if err != nil {
		fmt.Println(err.Error())
	}

	wg.Add(1)
	go StartWebServer(webPort, &wg)
	//go StartWebServer(webPort)

	//set end user defined minimum plays or default
	minimumPlayer, err := strconv.Atoi(args[2]) //2
	if err != nil {
		fmt.Println(err.Error())
	}
	if minimumPlayer > 0 {
		MinimumGameEntry = minimumPlayer
	}

	//Add Users and Make a play for each
	userchan := make(chan string)

	//start scores message broad caster channel
	go scoreMQbroadCaster()
	//start leaderboard message broadcaster channel
	go leadBoardMQbroadCaster()

	//wg.Add(1)
	//ADD FAKE USERS ALONG WITH NORMAL ONES
	if len(args) > 3 {
		FakePlayers = true
		totalUsers := 5
		go fakeUsersPlay(totalUsers)
	}
	gameChan := InitiateGame(gameCount)
	//for i := 0; ; i++ {
	MQ.enQ(msg.Wrap("GenMQ", "Total games played "+strconv.Itoa(gameChan)))
	Delay(5, "s")

	wg.Wait()
	close(userchan)

}

/*
::::::::::::::::::::::::::::WEB SERVER CODE:::::::::::::::::::::::::::::::::::::::::
::::::::::::::::::::::::::::::::::::::::::::::::::::::::::::::::::::::::::::::::::::
::::::::::::::::::::::::::::::::::::::::::::::::::::::::::::::::::::::::::::::::::::
*/
func RunError(err error) {
	if err != nil {
		fmt.Println(err.Error())
	}
}

var upgrader = websocket.Upgrader{
	ReadBufferSize:  1024,
	WriteBufferSize: 1024,
}

//WriteMsgType - socket mesage type
var WriteMsgType int

//StartWebServer - Will start the webserver on given port
func StartWebServer(port int, wg *sync.WaitGroup) {
	//func StartWebServer(port int) {

	defer wg.Done()
	//SITE USER INTERFACE ENDPOINTS
	ports := ":" + strconv.Itoa(port)
	//ports := strconv.Itoa(port)
	//ports = ":" + ports

	http.HandleFunc("/", servehome)
	http.HandleFunc("/ws", servews)

	http.Handle("/assets/", http.StripPrefix("/assets/", http.FileServer(http.Dir("assets"))))

	log.Println("Web server running on port " + ports)
	log.Fatal(http.ListenAndServe(ports, nil))

	//log.Println("Website running on port " + port)
	//http.ListenAndServe(port, nil)
}

func reader(conn *websocket.Conn) {
	msg := new(Message)
	//go socketBCast()

	for {
		messageType, p, err := conn.ReadMessage()
		if err != nil {
			fmt.Println(err.Error())
			return
		}
		WriteMsgType = messageType
		//process message from client instead of just logging
		log.Println(string(p))
		msgparam := strings.Split(string(p), "====")
		if len(msgparam) > 1 {
			PlayerName := msgparam[0]
			entry1, err := strconv.Atoi(msgparam[1])
			RunError(err)
			entry2, err := strconv.Atoi(msgparam[2])
			RunError(err)
			playerID := AddUser(PlayerName)

			//add player

			var totalScore int = 0
			// TODO: check for empty or invalid entries
			// TODO: sort entries for ease of use for threshold marker
			if entry1 > entry2 {
				entry1, entry2 = entry2, entry1
			}

			ent := [3]int{int(entry1), int(entry2), int(totalScore)}
			AddPlay(playerID, PlayerName, ent)

			MQ.enQ(msg.Wrap("GenMQ", "Total plays made so far: "+strconv.Itoa(len(Plays))))
			Delay(1, "s")
		}

		//fmt.Printf("Message from socket client =======> %s \n", string(p))
	}

}

func writeMessageToSock(conn *websocket.Conn, msg []byte) (int, error) {
	//fmt.Println("Sent message to socket client", string(msg))
	//Mutlock.Lock()
	var msgType int
	for cc, mt := range clientsConn {
		if cc == conn {
			msgType = mt
		}
	}
	if err := conn.WriteMessage(msgType, msg); err != nil {
		log.Println(err)
		return 0, err
	}
	//Mutlock.Unlock()
	return 1, nil
}

//servebtapp will return the static page for bt app
func servews(w http.ResponseWriter, r *http.Request) {
	upgrader.CheckOrigin = func(r *http.Request) bool {
		return true
	}
	ws, err := upgrader.Upgrade(w, r, nil)
	RunError(err)
	log.Printf("Socket Clients connected succesfully")

	defer ws.Close()

	// Register our new client
	Mutlock.Lock()
	clients[ws] = true
	Mutlock.Unlock()
	//reader(ws)
	go socketBCast()
	for {
		var msg Message
		// Read in a new message as JSON and map it to a Message object
		//Mutlock.Lock()
		messageType, p, err := ws.ReadMessage()
		if err != nil {
			log.Printf("error: %v", err)
			delete(clients, ws)
			break
		}
		//Mutlock.Unlock()

		//err := ws.ReadJSON(&msg)
		WriteMsgType = messageType
		clientsConn[ws] = WriteMsgType
		MQ.enQ(msg.Wrap("TotalclientsMQ", "Total observers:"+strconv.Itoa(len(clientsConn))))

		//process message from client instead of just logging
		log.Println(string(p))
		msgparam := strings.Split(string(p), "====")
		if len(msgparam) > 1 {
			PlayerName := msgparam[0]
			entry1, err := strconv.Atoi(msgparam[1])
			RunError(err)
			entry2, err := strconv.Atoi(msgparam[2])
			RunError(err)
			playerID := AddUser(PlayerName)

			//add player

			var totalScore int = 0
			// TODO: check for empty or invalid entries
			// TODO: sort entries for ease of use for threshold marker
			if entry1 > entry2 {
				entry1, entry2 = entry2, entry1
			}

			ent := [3]int{int(entry1), int(entry2), int(totalScore)}
			AddPlay(playerID, PlayerName, ent)

			MQ.enQ(msg.Wrap("GenMQ", "Total plays made so far: "+strconv.Itoa(len(Plays))))
			//broadcast <- msg
			//Delay(1, "s")
		}

		// Send the newly received message to the broadcast channel
		//broadcast <- msg
	}

	//start general message broad caster channel
	//go broadCaster()
	// Make sure we close the connection when the function returns

}

//socketBCast - live channel broadcast to web sockets
func socketBCast() {
	var p string

	for {
		p = MQ.Broadcast()
		if p != "" && len(clients) > 0 {

			for client, v := range clients {
				//err := client.WriteJSON(msg)
				if v == true && client != nil {

					Mutlock.Lock()
					resp, err := writeMessageToSock(client, []byte(p))
					if err != nil {
						log.Printf("Failed writting to socket broadcast Error(s-): %v", err)
					}
					log.Printf("%v Message(s-) wirttern to socket broadcast\n", resp)
					Mutlock.Unlock() //
				} else {
					//log.Printf("error: %v", err)
					client.Close()
					delete(clients, client)
					delete(clientsConn, client)
				}

			}

		}
		Delay(10, "ms")
	}

}

func serveSingle(pattern string, filename string) {
	http.HandleFunc(pattern, func(w http.ResponseWriter, r *http.Request) {
		http.ServeFile(w, r, filename)
	})
}

//servehome will return the static home page html file
func servehome(w http.ResponseWriter, r *http.Request) {
	http.ServeFile(w, r, "index.html")
}

/*
::::::::::::::::::::::::::::END OF WEB SERVER CODE:::::::::::::::::::::::::::::::::::::::::
::::::::::::::::::::::::::::::::::::::::::::::::::::::::::::::::::::::::::::::::::::
::::::::::::::::::::::::::::::::::::::::::::::::::::::::::::::::::::::::::::::::::::
*/
