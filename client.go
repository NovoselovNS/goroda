package main

import (
	"fmt"
	"net/rpc"
	"os"
)

func PlayerLossCheck(c *rpc.Client,name string){
	var ans bool
	err:= c.Call("Server.PlayerLossCheck",name, &ans);if err != nil {panic(err)}
	if !ans {//сервер отвечает false если игрок проигрывает из за того что просрочил время
		println("Вы просрочили время и выбываете из игры")
		os.Exit(0);
	}
}

func main() {
	server:="127.0.0.1:9999"
	if len(os.Args)>1 {server=os.Args[1]}
	c, err := rpc.Dial("tcp", server);if err != nil {panic(err);}
	var mySessionId int
	var name string
	for {
		print("Введите имя: ")
		fmt.Scanf("%s", &name)
		if len(name)==0{continue}
		err = c.Call("Server.RegisterPlayer", name, &mySessionId);if err != nil {panic(err)}
		if mySessionId != -1 {break} else {
			println("С таким именем уже есть игрок")
		}
	}

	println("Вы вошли в сессию № ", mySessionId)
	var players []string
	err = c.Call("Server.GetPlayers", mySessionId, &players);if err != nil {panic(err);}
	if len(players)>=2{
		print("В сессии сейчас есть игроки: ")
		for _,p:=range players{if p!=name{print(p," ")}}
		println()
	}

	for {
		var newPlayer string
		err = c.Call("Server.WaitForPlayers", name, &newPlayer);if err != nil {panic(err);}
		if newPlayer == "" {break}//сервер присылает пустой ответ, если уже набрано нужное количество игроков
		println("Присоединился новый игрок: ", newPlayer)
	}

	println("Игра начинается")
	for {
		var whoseMove, answer string
		err = c.Call("Server.WhooseMove", mySessionId, &whoseMove);if err != nil {panic(err)}
		if whoseMove == "" {//сервер высылает пустой ответ в случае победы
			println("Вы победили, игра закончена")
			os.Exit(0)
		}
		if whoseMove == name {//если ход клиента
			go PlayerLossCheck(c, name) //обрабатывает случай проигрыша
			for {
				print("Ваш ход: ")
				fmt.Scanf("%s", &answer)
				var serverReply string
				err = c.Call("Server.TryAnswer", []string{name,answer}, &serverReply);if err != nil {panic(err)}
				if serverReply == "" {break} else {//сервер высылает пустой ответ в случае принятия
					println(serverReply)//иначе печать причины  (напр. такое слово уже было)
				}
			}
		} else { //случай, когда ход другого игрока
			print(whoseMove, ": ")
			err = c.Call("Server.GetAnswer", name, &answer);if err != nil {panic(err)}
			println(answer)//сервер пришлет ответ другого игрока или информацию о том что он просрочил время
		}
	}
}
