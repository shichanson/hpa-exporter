package metrics

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"time"

	as_v1 "k8s.io/api/autoscaling/v1"
	as_v2 "k8s.io/api/autoscaling/v2beta1"
	core_v1 "k8s.io/api/core/v1"
	meta_v1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	"github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/aws/session"
	"github.com/aws/aws-sdk-go/service/cloudwatchlogs"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/shichanson/hpa-exporter/pkg/setting"
)

const (
	defaultMetricsInterval  = 30
	defaultConditionLogging = false
	defaultLoggingTo        = "stdout"
	defaultCWLogGroup       = "hpa-exporter"
	defaultCWLogStream      = "condition-log"
	defaultLoggingInterval  = 60
	defaultAddr             = ":9296"
)

const RootDoc = `<html>
<head><title>HPA Exporter</title></head>
<body>
<h1>HPA Exporter</h1>
<p><a href="/metrics">Metrics</a></p>
</body>
</html>
`

type conditions struct {
	Name       string                                   `json:"name"`
	Conditions []as_v2.HorizontalPodAutoscalerCondition `json:"conditions"`
}

type commonMetrics struct {
	Kind       string
	Name       string
	MetricName string
	Value      float64
}

var Addr = flag.String("listen-address", defaultAddr, "The address to listen on for HTTP requests.")
var MetricsInterval = flag.Int("MetricsInterval", defaultMetricsInterval, "Interval to scrape HPA status.")
var LoggingInterval = flag.Int("loggingInterval", defaultLoggingInterval, "Interval to logging HPA conditions.")
var ConditionLogging = flag.Bool("conditionLogging", defaultConditionLogging, "Logging HPA conditions.")
var LoggingTo = flag.String("loggingTo", defaultLoggingTo, "Where to log. (stdout or cwlogs)")
var cwLogGroup = flag.String("cwLogGroup", defaultCWLogGroup, "Name of CWLog group.")
var cwLogStream = flag.String("cwLogStream", defaultCWLogStream, "Name of CWLog stream.")

var kubeClient = setting.KubeClient

var cwSession = func() *cloudwatchlogs.CloudWatchLogs {
	sess := session.Must(session.NewSessionWithOptions(session.Options{
		SharedConfigState: session.SharedConfigEnable,
	}))
	return cloudwatchlogs.New(sess)
}()

var baseLabels = []string{
	"hpa_name",
	"hpa_namespace",
	"ref_kind",
	"ref_name",
	"ref_apiversion",
}

var metricLabels = []string{
	"metric_kind",
	"metric_name",
	"metric_metricname",
}

var annoLabels = []string{
	"cond_status",
	"cond_reason",
	"cond_message",
}

var (
	HpaCurrentPodsNum = prometheus.NewGaugeVec(
		prometheus.GaugeOpts{
			Name: "hpa_current_pods_num",
			Help: "Number of current pods by status.",
		},
		baseLabels,
	)

	HpaDesiredPodsNum = prometheus.NewGaugeVec(
		prometheus.GaugeOpts{
			Name: "hpa_desired_pods_num",
			Help: "Number of desired pods by status.",
		},
		baseLabels,
	)

	HpaMinPodsNum = prometheus.NewGaugeVec(
		prometheus.GaugeOpts{
			Name: "hpa_min_pods_num",
			Help: "Number of min pods by spec.",
		},
		baseLabels,
	)

	HpaMaxPodsNum = prometheus.NewGaugeVec(
		prometheus.GaugeOpts{
			Name: "hpa_max_pods_num",
			Help: "Number of max pods by spec.",
		},
		baseLabels,
	)

	HpaLastScaleSecond = prometheus.NewGaugeVec(
		prometheus.GaugeOpts{
			Name: "hpa_last_scale_second",
			Help: "Time the scale was last executed.",
		},
		baseLabels,
	)

	HpaCurrentMetricsValue = prometheus.NewGaugeVec(
		prometheus.GaugeOpts{
			Name: "hpa_current_metrics_value",
			Help: "Current Metrics Value.",
		},
		append(baseLabels, metricLabels...),
	)

	HpaTargetMetricsValue = prometheus.NewGaugeVec(
		prometheus.GaugeOpts{
			Name: "hpa_target_metrics_value",
			Help: "Target Metrics Value.",
		},
		append(baseLabels, metricLabels...),
	)

	HpaAbleToScale = prometheus.NewGaugeVec(
		prometheus.GaugeOpts{
			Name: "hpa_able_to_scale",
			Help: "status able to scale from annotation.",
		},
		append(baseLabels, annoLabels...),
	)

	HpaScalingActive = prometheus.NewGaugeVec(
		prometheus.GaugeOpts{
			Name: "hpa_scaling_active",
			Help: "status scaling active from annotation.",
		},
		append(baseLabels, annoLabels...),
	)

	HpaScalingLimited = prometheus.NewGaugeVec(
		prometheus.GaugeOpts{
			Name: "hpa_scaling_limited",
			Help: "status scaling limited from annotation.",
		},
		append(baseLabels, annoLabels...),
	)
)

var collectors = []prometheus.Collector{
	HpaCurrentPodsNum,
	HpaDesiredPodsNum,
	HpaMinPodsNum,
	HpaMaxPodsNum,
	HpaLastScaleSecond,
	HpaCurrentMetricsValue,
	HpaTargetMetricsValue,
	HpaAbleToScale,
	HpaScalingActive,
	HpaScalingLimited,
}

func init() {
	prometheus.MustRegister(collectors...)
}

func ResetAllMetric() {
	for _, c := range collectors {
		if v, ok := c.(*prometheus.GaugeVec); ok {
			v.Reset()
		}
	}
}

func ValidateFlags() error {
	if !(*LoggingTo == "stdout" || *LoggingTo == "cwlogs") {
		return fmt.Errorf("invalid value `%s` of flag `loggingTo`, specify either `stdout` or `cwlogs`", *LoggingTo)
	}
	return nil
}

func getHpaList() ([]as_v1.HorizontalPodAutoscaler, error) {
	out, err := kubeClient.AutoscalingV1().HorizontalPodAutoscalers("").List(context.TODO(),meta_v1.ListOptions{})
	return out.Items, err
}

func GetHpaListV2() ([]as_v2.HorizontalPodAutoscaler, error) {
	out, err := kubeClient.AutoscalingV2beta1().HorizontalPodAutoscalers("").List(context.TODO(),meta_v1.ListOptions{})
	return out.Items, err
}

func MergeLabels(m1, m2 map[string]string) map[string]string {
	ans := map[string]string{}

	for k, v := range m1 {
		ans[k] = v
	}
	for k, v := range m2 {
		ans[k] = v
	}
	return (ans)
}

func MakeAnnotationCondLabels(cond as_v2.HorizontalPodAutoscalerCondition) (prometheus.Labels, prometheus.Labels) {
	labelForward := prometheus.Labels{
		"cond_status":  fmt.Sprintf("%v", cond.Status),
		"cond_reason":  cond.Reason,
		"cond_message": cond.Message,
	}
	var statusReverse string
	if cond.Status == core_v1.ConditionTrue {
		statusReverse = fmt.Sprintf("%v", core_v1.ConditionFalse)
	} else {
		statusReverse = fmt.Sprintf("%v", core_v1.ConditionTrue)
	}
	labelReverse := prometheus.Labels{
		"cond_status":  statusReverse,
		"cond_reason":  "",
		"cond_message": "",
	}

	return labelForward, labelReverse
}

func ParseObjectSpec(m *as_v2.ObjectMetricSource) commonMetrics {
	return commonMetrics{
		Kind:       m.Target.Kind,
		Name:       m.Target.Name,
		MetricName: m.MetricName,
		Value:      float64(m.TargetValue.MilliValue()) / 1000,
	}
}

func ParsePodsSpec(m *as_v2.PodsMetricSource) commonMetrics {
	return commonMetrics{
		Kind:       "Pod",
		Name:       "-",
		MetricName: m.MetricName,
		Value:      float64(m.TargetAverageValue.MilliValue()) / 1000,
	}
}

func ParseResourceSpec(m *as_v2.ResourceMetricSource) commonMetrics {
	var t float64
	if m.TargetAverageUtilization == nil {
		t = float64(m.TargetAverageValue.MilliValue()) / 1000
	} else {
		t = float64(*m.TargetAverageUtilization)
	}
	return commonMetrics{
		Kind:       "Resource",
		Name:       m.Name.String(),
		MetricName: "-",
		Value:      t,
	}
}

func ParseExternalSpec(m *as_v2.ExternalMetricSource) commonMetrics {
	var t float64
	if m.TargetAverageValue == nil {
		t = float64(m.TargetValue.MilliValue()) / 1000
	} else {
		t = float64(m.TargetAverageValue.MilliValue()) / 1000
	}
	return commonMetrics{
		Kind:       "External",
		Name:       "-",
		MetricName: m.MetricName,
		Value:      t,
	}
}

func ParseObjectStatus(m *as_v2.ObjectMetricStatus) commonMetrics {
	return commonMetrics{
		Kind:       m.Target.Kind,
		Name:       m.Target.Name,
		MetricName: m.MetricName,
		Value:      float64(m.CurrentValue.MilliValue()) / 1000,
	}
}

func ParsePodsStatus(m *as_v2.PodsMetricStatus) commonMetrics {
	return commonMetrics{
		Kind:       "Pod",
		Name:       "-",
		MetricName: m.MetricName,
		Value:      float64(m.CurrentAverageValue.MilliValue()) / 1000,
	}
}

func ParseResourceStatus(m *as_v2.ResourceMetricStatus) commonMetrics {
	var t float64
	if m.CurrentAverageUtilization == nil {
		t = float64(m.CurrentAverageValue.MilliValue()) / 1000
	} else {
		t = float64(*m.CurrentAverageUtilization)
	}
	return commonMetrics{
		Kind:       "Resource",
		Name:       m.Name.String(),
		MetricName: "-",
		Value:      t,
	}
}

func ParseExternalStatus(m *as_v2.ExternalMetricStatus) commonMetrics {
	var t float64
	if m.CurrentAverageValue == nil {
		t = float64(m.CurrentValue.MilliValue()) / 1000
	} else {
		t = float64(m.CurrentAverageValue.MilliValue()) / 1000
	}
	return commonMetrics{
		Kind:       "External",
		Name:       "-",
		MetricName: m.MetricName,
		Value:      t,
	}
}

func ParseCommonMetrics(m commonMetrics) (float64, prometheus.Labels) {
	return m.Value, prometheus.Labels{
		"metric_kind":       m.Kind,
		"metric_name":       m.Name,
		"metric_metricname": m.MetricName,
	}
}

func PutHPAConditionToCWLog(hpa []as_v2.HorizontalPodAutoscaler) error {
	t, e := token()
	if e != nil {
		return e
	}
	cwevent := []*cloudwatchlogs.InputLogEvent{}
	timestamp := aws.Int64(time.Now().Unix() * 1000)
	for _, a := range hpa {
		s := HpaConditionJsonString(a)
		cwevent = append(cwevent, &cloudwatchlogs.InputLogEvent{
			Message:   aws.String(s),
			Timestamp: timestamp,
		})
	}
	putEvent := &cloudwatchlogs.PutLogEventsInput{
		LogEvents:     cwevent,
		LogGroupName:  cwLogGroup,
		LogStreamName: cwLogStream,
		SequenceToken: t,
	}
	//return contains only token `ret["NextSequenceToken"]`
	_, err := cwSession.PutLogEvents(putEvent)
	return err
}

func HpaConditionJsonString(hpa as_v2.HorizontalPodAutoscaler) string {
	cond := conditions{
		Name:       hpa.ObjectMeta.Name,
		Conditions: hpa.Status.Conditions,
	}
	jsonBytes, err := json.Marshal(cond)
	if err != nil {
		fmt.Println("JSON Marshal error:", err)
		return "{}"
	}
	return string(jsonBytes)
}

func token() (token *string, err error) {
	input := &cloudwatchlogs.DescribeLogStreamsInput{
		LogGroupName:        cwLogGroup,
		LogStreamNamePrefix: cwLogStream,
	}
	x, err := cwSession.DescribeLogStreams(input)
	if err == nil {
		if len(x.LogStreams) == 0 {
			err = createStream()
		} else {
			token = x.LogStreams[0].UploadSequenceToken
		}
	}
	return
}

func CheckLogGroup() error {
	input := &cloudwatchlogs.DescribeLogGroupsInput{
		LogGroupNamePrefix: cwLogGroup,
	}
	if r, e := cwSession.DescribeLogGroups(input); e == nil {
		if len(r.LogGroups) == 0 {
			if e := createLogGroup(); e != nil {
				return e
			}
		}
	} else {
		return e
	}
	return nil
}

func createLogGroup() error {
	input := &cloudwatchlogs.CreateLogGroupInput{
		LogGroupName: cwLogGroup,
	}
	_, err := cwSession.CreateLogGroup(input)
	return err
}

func createStream() error {
	input := &cloudwatchlogs.CreateLogStreamInput{
		LogGroupName:  cwLogGroup,
		LogStreamName: cwLogStream,
	}
	_, err := cwSession.CreateLogStream(input)
	return err
}