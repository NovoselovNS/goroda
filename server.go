package main

import (
	"encoding/json"
	"io/ioutil"
	"math/rand"
	"net"
	"net/rpc"
	"strings"
	"sync"
	"time"
)

const maxPlayers = 3 //число игроков в сессии
const maxTime = 15*time.Second //время ожидания ответа от игроков
var Cities []string
var m sync.Mutex

type Session struct{
	wordsUsed     []string        //какие слова были использованы во время игры
	players       []string        //имена игроков
	whoseMove     string          //имя ведущего игрока
	moveBegun     time.Time       //время начала хода
	plc           chan int         //канал для отработки случая когда игрок просрочил время
	kickedPlayers map[string]bool //какие из игроков уже выбыли
}

var sess int //в какую сессию попадет новый игрок
var sessions []Session
var players   map[string]int	//дает возможность определить какой игрок в какой сессии
var playerChan map[string]chan string	//канал для передачи каждому игроку

func sendChan(name string,word string) {
	playerChan[name]<-word
}

type Server struct {}

func (this *Server) RegisterPlayer(name string, reply *int) error {
	m.Lock()
	defer m.Unlock()
	for p:=range players{if p==name{*reply=-1;return nil}}//проверка что игрока с этим именем нет
	players[name]= sess
	playerChan[name]=make(chan string)
	if len(sessions)-1< sess { //инициализация структуры
		sessions =append(sessions, Session{})
		sessions[players[name]].plc =make(chan int)
		sessions[players[name]].kickedPlayers=make(map[string]bool)
	}
	for _,p:=range sessions[players[name]].players{
		if p!=name{
			go sendChan(p,name) //рассылаем всем игрокам из данного сессия имя нового участника
		}
	}

	sessions[sess].players=append(sessions[sess].players,name)
	*reply= sess

	if len(sessions[sess].players)>=maxPlayers{ //проверка заполненности комнаты
		for _, p:= range sessions[sess].players {
			go sendChan(p,"")//рассылаем всем игрокам из данного сессия, вместо имени пустое сообщение (интерпретируется клиентом как готовность комнаты)
		}
		changeWhose(name)	//определяем очердность хода
		sess++
	}
	return nil
}

func (this *Server) WhooseMove(id int,whoseMove *string) error {
	m.Lock() //высылает имя игрока, чей ход
	defer m.Unlock()
	*whoseMove = sessions[id].whoseMove
	return nil
}

func (this *Server) PlayerLossCheck(name string,ans *bool) error {//функция возвращает false если игрок просрочит время
	select {//задействует первый готовый канал
	case <- sessions[players[name]].plc: *ans=true //этот канал будет готов в момент ответа
	case <- time.After(maxTime): 	//этот канал будет готов по истечении времени
		*ans=false
		sessions[players[name]].kickedPlayers[name]=true //отмечаем игрока как выбывшего, он не будет выбираться при очередности хода
		changeWhose(name)                                //определяем очередность хода
		for _,v:=range sessions[players[name]].players{
			if name!=v {go sendChan(v," просрочил время")}//рассылаем всем игрокам ответ, после этого клиенты будут получать очередность
		}
	}
	return nil
}

func (this *Server) WaitForPlayers(name string, newPlayer *string) error {
	*newPlayer = <- playerChan[name] //ожидает имя игрока из канала и присылает по готовности
	return nil
}

func (this *Server) GetAnswer(name string,ans *string) error {
	*ans = <- playerChan[name]
	return nil
}

func changeWhose(name string){
	i:=players[name]
	var t int
	if len(sessions[i].kickedPlayers)!=maxPlayers-1{ //проверка что не все, кроме одного, игроки выбыли
		for{
			t=rand.Int()%len(sessions[i].players)
			if sessions[i].kickedPlayers[sessions[i].players[t]]==true{continue} //проверка что игрок не выбыл
			if sessions[i].whoseMove != sessions[i].players[t]{break} //проверка, что следующий игрок другой
		}
		sessions[i].whoseMove = sessions[i].players[t]
		sessions[i].moveBegun =time.Now()
	}else{ //если выбыли все игроки, то остался победитель
		sessions[i].whoseMove ="" //интерпретируется клиентом как победа
	}
}

func (this *Server) TryAnswer(ans []string,reply *string) error {
	m.Lock()
	defer m.Unlock()
	name:=ans[0]
	city:=ans[1]
	id:=players[name]
	l:=&sessions[id]
	if len(city)<=2{
		*reply="Слово слишком короткое"
		return nil
	}

	exists:=false
	for _,v:=range Cities{
		if strings.ToLower(v)==strings.ToLower(city){exists=true;break}
	}
	if !exists{
		*reply="Нет такого города"
		return nil
	}
	exists=false
	for _,v:=range l.wordsUsed {
		if v==city{exists=true;break}
	}
	if exists{
		*reply="Такое слово уже было названо"
		return nil
	}

	if len(l.wordsUsed)!=0 && !cmpCities(l.wordsUsed[len(l.wordsUsed)-1],city){
		*reply="Начало слова не совпадает с окончанием предыдущего"
		return nil
	}

	if time.Now().Sub(l.moveBegun)<maxTime{
		//слово принято
		l.plc <-0 //PlayerLossCheck() завершается
		l.wordsUsed =append(l.wordsUsed,city)
		for _,v:=range sessions[players[name]].players{
			if name!=v {go sendChan(v,city)}//рассылаем всем игрокам ответ
		}
		changeWhose(name)
	}
	//если слово не принято по причине просрочки, ситуация будет обработана в PlayerLossCheck
	return nil
}

func cmpCities(a string,b string)bool{
	A:=[]rune(strings.ToLower(a))
	B:=[]rune(strings.ToLower(b))
	if A[len(A)-1]=='ь' {
		return A[len(A)-2]==B[0]
	}
	return A[len(A)-1]==B[0]
}

func (this *Server) GetPlayers(id int, reply *[]string) error {//высылает список игроков, относящихся к сессия
	m.Lock()
	defer m.Unlock()
	*reply= sessions[id].players
	return nil
}

func main() {
	err:= rpc.Register(new(Server));if err != nil {panic(err)}
	ln, err:= net.Listen("tcp", ":9999");if err != nil {panic(err)}
	for {
		conn, err := ln.Accept();if err != nil {panic(err)}
		go rpc.ServeConn(conn)
	}
}

func init(){
	rand.Seed(time.Now().Unix())
	players=make(map[string]int)
	playerChan=make(map[string]chan string)

	var tt struct{
		City []struct{
			City_id string
			Country_id string
			Region_id string
			Name string
		}
	}
	jsonFile, err := ioutil.ReadFile("cities.json");if err!=nil {panic(err)}
	err= json.Unmarshal([]byte(jsonFile), &tt);if err!=nil {panic(err)}
	for _,v:=range tt.City{Cities=append(Cities,v.Name)}
}
