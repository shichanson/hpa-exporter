module github.com/shichanson/hpa-exporter

go 1.15

replace (
	github.com/shichanson/hpa-exporter/conf => ./conf
	github.com/shichanson/hpa-exporter/pkg/setting => ./pkg/setting

)

require (
	github.com/aws/aws-sdk-go v1.36.7
	github.com/go-ini/ini v1.62.0
	github.com/mitchellh/go-homedir v1.1.0
	github.com/prometheus/client_golang v1.8.0
	github.com/prometheus/common v0.15.0
	gopkg.in/ini.v1 v1.62.0 // indirect
	k8s.io/api v0.0.0-20201209045733-fcac651617f2
	k8s.io/apimachinery v0.0.0-20201209085528-15c5dba13c59
	k8s.io/client-go v0.0.0-20201210210011-77dfe4dff7d7
)
