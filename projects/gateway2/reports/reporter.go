package reports

import (
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"
	gwv1 "sigs.k8s.io/gateway-api/apis/v1"
)

type ReportMap struct {
	gateways map[types.NamespacedName]*GatewayReport
	routes   map[types.NamespacedName]*RouteReport
}

type GatewayReport struct {
	conditions         []metav1.Condition
	listeners          map[string]*ListenerReport
	observedGeneration int64
}

type ListenerReport struct {
	Status gwv1.ListenerStatus
}

type RouteReport struct {
	parents            map[ParentRefKey]*ParentRefReport
	observedGeneration int64
}

type ParentRefReport struct {
	Conditions []metav1.Condition
}

type ParentRefKey struct {
	Group string
	Kind  string
	types.NamespacedName
}

func NewReportMap() ReportMap {
	gr := make(map[types.NamespacedName]*GatewayReport)
	rr := make(map[types.NamespacedName]*RouteReport)
	return ReportMap{
		gateways: gr,
		routes:   rr,
	}
}

// Exported for unit test, validation_test.go can be refactored to reduce this visibility
func (r *ReportMap) Gateway(gateway *gwv1.Gateway) *GatewayReport {
	key := client.ObjectKeyFromObject(gateway)
	gr := r.gateways[key]
	if gr == nil {
		gr = &GatewayReport{}
		gr.observedGeneration = gateway.Generation
		r.gateways[key] = gr
	}
	return gr
}

func (r *ReportMap) route(route *gwv1.HTTPRoute) *RouteReport {
	key := client.ObjectKeyFromObject(route)
	rr := r.routes[key]
	if rr == nil {
		rr = &RouteReport{}
		rr.observedGeneration = route.Generation
		r.routes[key] = rr
	}
	return rr
}

func (g *GatewayReport) Listener(listener *gwv1.Listener) ListenerReporter {
	return g.listener(listener)
}

func (g *GatewayReport) listener(listener *gwv1.Listener) *ListenerReport {
	if g.listeners == nil {
		g.listeners = make(map[string]*ListenerReport)
	}
	lr := g.listeners[string(listener.Name)]
	if lr == nil {
		lr = NewListenerReport(string(listener.Name))
		g.listeners[string(listener.Name)] = lr
	}
	return lr
}

func (g *GatewayReport) GetConditions() []metav1.Condition {
	if g == nil {
		return []metav1.Condition{}
	}
	return g.conditions
}

func (g *GatewayReport) SetCondition(gc GatewayCondition) {
	condition := metav1.Condition{
		Type:    string(gc.Type),
		Status:  gc.Status,
		Reason:  string(gc.Reason),
		Message: gc.Message,
	}
	g.conditions = append(g.conditions, condition)
}

func NewListenerReport(name string) *ListenerReport {
	lr := ListenerReport{}
	lr.Status.Name = gwv1.SectionName(name)
	return &lr
}

func (l *ListenerReport) SetCondition(lc ListenerCondition) {
	condition := metav1.Condition{
		Type:    string(lc.Type),
		Status:  lc.Status,
		Reason:  string(lc.Reason),
		Message: lc.Message,
	}
	l.Status.Conditions = append(l.Status.Conditions, condition)
}

func (l *ListenerReport) SetSupportedKinds(rgks []gwv1.RouteGroupKind) {
	l.Status.SupportedKinds = rgks
}

func (l *ListenerReport) SetAttachedRoutes(n uint) {
	l.Status.AttachedRoutes = int32(n)
}

type reporter struct {
	report *ReportMap
}

func (r *reporter) Gateway(gateway *gwv1.Gateway) GatewayReporter {
	return r.report.Gateway(gateway)
}

func (r *reporter) Route(route *gwv1.HTTPRoute) HTTPRouteReporter {
	return r.report.route(route)
}

func getParentRefKey(parentRef *gwv1.ParentReference) ParentRefKey {
	var kind string
	if parentRef.Kind != nil {
		kind = string(*parentRef.Kind)
	}
	var ns string
	if parentRef.Namespace != nil {
		kind = string(*parentRef.Namespace)
	}
	return ParentRefKey{
		Group: string(parentRef.Name),
		Kind:  kind,
		NamespacedName: types.NamespacedName{
			Namespace: ns,
			Name:      string(parentRef.Name),
		},
	}
}

func (r *RouteReport) parentRef(parentRef *gwv1.ParentReference) *ParentRefReport {
	key := getParentRefKey(parentRef)
	if r.parents == nil {
		r.parents = make(map[ParentRefKey]*ParentRefReport)
	}
	var prr *ParentRefReport
	prr, ok := r.parents[key]
	if !ok {
		prr = &ParentRefReport{}
		r.parents[key] = prr
	}
	return prr
}

func (r *RouteReport) ParentRef(parentRef *gwv1.ParentReference) ParentRefReporter {
	return r.parentRef(parentRef)
}

func (prr *ParentRefReport) SetCondition(rc HTTPRouteCondition) {
	condition := metav1.Condition{
		Type:    string(rc.Type),
		Status:  rc.Status,
		Reason:  string(rc.Reason),
		Message: rc.Message,
	}
	prr.Conditions = append(prr.Conditions, condition)
}

func NewReporter(reportMap *ReportMap) Reporter {
	return &reporter{report: reportMap}
}

type Reporter interface {
	// returns the object reporter for the given type
	Gateway(gateway *gwv1.Gateway) GatewayReporter
	Route(route *gwv1.HTTPRoute) HTTPRouteReporter
}

type GatewayReporter interface {
	Listener(listener *gwv1.Listener) ListenerReporter
	SetCondition(condition GatewayCondition)
}

type ListenerReporter interface {
	SetCondition(ListenerCondition)
	SetSupportedKinds([]gwv1.RouteGroupKind)
	SetAttachedRoutes(n uint)
}

type HTTPRouteReporter interface {
	ParentRef(parentRef *gwv1.ParentReference) ParentRefReporter
}

type ParentRefReporter interface {
	SetCondition(condition HTTPRouteCondition)
}

type GatewayCondition struct {
	Type    gwv1.GatewayConditionType
	Status  metav1.ConditionStatus
	Reason  gwv1.GatewayConditionReason
	Message string
}

type ListenerCondition struct {
	Type    gwv1.ListenerConditionType
	Status  metav1.ConditionStatus
	Reason  gwv1.ListenerConditionReason
	Message string
}

type HTTPRouteCondition struct {
	Type    gwv1.RouteConditionType
	Status  metav1.ConditionStatus
	Reason  gwv1.RouteConditionReason
	Message string
}
