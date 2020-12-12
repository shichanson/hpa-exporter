package setting

import (
	"github.com/go-ini/ini"
	"log"
)

var (
	Kubeconfig *string
	Cfg *ini.File
)

func init(){

}

func LoadBase(){
	var err error
	Cfg,err = ini.Load("./conf/app.ini")
	if err != nil{
		log.Fatalf("Fail to parse 'conf/app.ini': %v", err)
	}
}