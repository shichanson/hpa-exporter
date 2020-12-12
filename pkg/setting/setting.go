package setting

import (
	"flag"
	"github.com/go-ini/ini"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/rest"
	"k8s.io/client-go/tools/clientcmd"
	"k8s.io/client-go/util/homedir"
	"log"
	"path/filepath"
)

var (
	Cfg *ini.File
	KubeClient *kubernetes.Clientset
)

func init(){
	LoadBase()
	LoadKubeClient()
}

func LoadBase(){
	var err error
	Cfg,err = ini.Load("./conf/app.ini")
	if err != nil{
		log.Fatalf("Fail to parse 'conf/app.ini': %v", err)
	}
}

func LoadKubeClient()  {
	if mode := Cfg.Section("").Key("RUN_MODE").MustString("out_cluster");mode == "out_cluster"{
		// creates the out-cluster kubeconfig
		var kubeconfig *string
		if home := homedir.HomeDir(); home != "" {
			kubeconfig = flag.String("kubeconfig", filepath.Join(home, ".kube", "config"), "(optional) absolute path to the kubeconfig file")
		} else {
			kubeconfig = flag.String("kubeconfig", "", "absolute path to the kubeconfig file")
		}
		flag.Parse()

		// use the current context in kubeconfig
		config, err := clientcmd.BuildConfigFromFlags("", *kubeconfig)
		if err != nil {
			panic(err.Error())
		}

		// create the clientset
		KubeClient, err = kubernetes.NewForConfig(config)
		if err != nil {
			panic(err.Error())
		}
	}else {
		// creates the in-cluster config
		config, err := rest.InClusterConfig()
		if err != nil {
			panic(err.Error())
		}
		// creates the clientset
		KubeClient, err = kubernetes.NewForConfig(config)
		if err != nil {
			panic(err.Error())
		}
	}

}

