package main

import (
	"flag"
	as_v2 "k8s.io/api/autoscaling/v2beta1"
	"net/http"
	"time"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promhttp"
	"github.com/prometheus/common/log"
	"github.com/shichanson/hpa-exporter/metrics"

)



func main() {
	flag.Parse()
	e := metrics.ValidateFlags()
	if e != nil {
		panic(e)
	}
	time.Local, e = time.LoadLocation("Asia/Tokyo")
	if e != nil {
		time.Local = time.FixedZone("Asia/Tokyo", 9*60*60)
	}

	if *metrics.ConditionLogging {
		e = metrics.CheckLogGroup()
		if e != nil {
			panic(e)
		}
	}

	log.Info("start HPA exporter")

	if *metrics.ConditionLogging {
		go func() {
			for {
				hpa, err := metrics.GetHpaListV2()
				if err != nil {
					log.Errorln(err)
					continue
				}
				if *metrics.LoggingTo == "cwlogs" {
					metrics.PutHPAConditionToCWLog(hpa)
				} else {
					for _, a := range hpa {
						log.Infoln(metrics.HpaConditionJsonString(a))
					}
				}
				time.Sleep(time.Duration(*metrics.LoggingInterval) * time.Second)
			}
		}()
	}

	go func() {
		for {
			hpa, err := metrics.GetHpaListV2()
			if err != nil {
				log.Errorln(err)
				continue
			}
			metrics.ResetAllMetric()
			for _, a := range hpa {
				baseLabel := prometheus.Labels{
					"hpa_name":       a.ObjectMeta.Name,
					"hpa_namespace":  a.ObjectMeta.Namespace,
					"ref_kind":       a.Spec.ScaleTargetRef.Kind,
					"ref_name":       a.Spec.ScaleTargetRef.Name,
					"ref_apiversion": a.Spec.ScaleTargetRef.APIVersion,
				}

				metrics.HpaCurrentPodsNum.With(baseLabel).Set(float64(a.Status.CurrentReplicas))
				metrics.HpaDesiredPodsNum.With(baseLabel).Set(float64(a.Status.DesiredReplicas))
				if a.Spec.MinReplicas != nil {
					metrics.HpaMinPodsNum.With(baseLabel).Set(float64(*a.Spec.MinReplicas))
				}
				metrics.HpaMaxPodsNum.With(baseLabel).Set(float64(a.Spec.MaxReplicas))
				if a.Status.LastScaleTime != nil {
					metrics.HpaLastScaleSecond.With(baseLabel).Set(float64(a.Status.LastScaleTime.Unix()))
				}

				for _, metric := range a.Spec.Metrics {
					switch metric.Type {
					case as_v2.ObjectMetricSourceType:
						m := metrics.ParseObjectSpec(metric.Object)
						v, l := metrics.ParseCommonMetrics(m)
						metrics.HpaTargetMetricsValue.With(metrics.MergeLabels(baseLabel, l)).Set(v)
					case as_v2.PodsMetricSourceType:
						m := metrics.ParsePodsSpec(metric.Pods)
						v, l := metrics.ParseCommonMetrics(m)
						metrics.HpaTargetMetricsValue.With(metrics.MergeLabels(baseLabel, l)).Set(v)
					case as_v2.ResourceMetricSourceType:
						m := metrics.ParseResourceSpec(metric.Resource)
						v, l := metrics.ParseCommonMetrics(m)
						metrics.HpaTargetMetricsValue.With(metrics.MergeLabels(baseLabel, l)).Set(v)
					case as_v2.ExternalMetricSourceType:
						m := metrics.ParseExternalSpec(metric.External)
						v, l := metrics.ParseCommonMetrics(m)
						metrics.HpaTargetMetricsValue.With(metrics.MergeLabels(baseLabel, l)).Set(v)
					default:
						continue
					}
				}

				for _, metric := range a.Status.CurrentMetrics {
					switch metric.Type {
					case as_v2.ObjectMetricSourceType:
						m := metrics.ParseObjectStatus(metric.Object)
						v, l := metrics.ParseCommonMetrics(m)
						metrics.HpaCurrentMetricsValue.With(metrics.MergeLabels(baseLabel, l)).Set(v)
					case as_v2.PodsMetricSourceType:
						m := metrics.ParsePodsStatus(metric.Pods)
						v, l := metrics.ParseCommonMetrics(m)
						metrics.HpaCurrentMetricsValue.With(metrics.MergeLabels(baseLabel, l)).Set(v)
					case as_v2.ResourceMetricSourceType:
						m := metrics.ParseResourceStatus(metric.Resource)
						v, l := metrics.ParseCommonMetrics(m)
						metrics.HpaCurrentMetricsValue.With(metrics.MergeLabels(baseLabel, l)).Set(v)
					case as_v2.ExternalMetricSourceType:
						m := metrics.ParseExternalStatus(metric.External)
						v, l := metrics.ParseCommonMetrics(m)
						metrics.HpaCurrentMetricsValue.With(metrics.MergeLabels(baseLabel, l)).Set(v)
					default:
						continue
					}
				}

				for _, cond := range a.Status.Conditions {
					annoLabel, annoLabelRev := metrics.MakeAnnotationCondLabels(cond)
					switch cond.Type {
					case as_v2.AbleToScale:
						metrics.HpaAbleToScale.With(metrics.MergeLabels(baseLabel, annoLabel)).Set(float64(1))
						metrics.HpaAbleToScale.With(metrics.MergeLabels(baseLabel, annoLabelRev)).Set(float64(0))
					case as_v2.ScalingActive:
						metrics.HpaScalingActive.With(metrics.MergeLabels(baseLabel, annoLabel)).Set(float64(1))
						metrics.HpaScalingActive.With(metrics.MergeLabels(baseLabel, annoLabelRev)).Set(float64(0))
					case as_v2.ScalingLimited:
						metrics.HpaScalingLimited.With(metrics.MergeLabels(baseLabel, annoLabel)).Set(float64(1))
						metrics.HpaScalingLimited.With(metrics.MergeLabels(baseLabel, annoLabelRev)).Set(float64(0))
					}
				}
			}
			time.Sleep(time.Duration(*metrics.MetricsInterval) * time.Second)
		}
	}()
	http.Handle("/metrics", promhttp.Handler())
	http.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte(metrics.RootDoc))
	})

	log.Fatal(http.ListenAndServe(*metrics.Addr, nil))
}
