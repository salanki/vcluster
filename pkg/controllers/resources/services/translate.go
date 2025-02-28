package services

import (
	"github.com/loft-sh/vcluster/pkg/controllers/syncer/translator"
	"github.com/loft-sh/vcluster/pkg/util/translate"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/equality"
)

func (s *serviceSyncer) translate(vObj *corev1.Service) *corev1.Service {
	newService := s.TranslateMetadata(vObj).(*corev1.Service)
	RewriteSelector(newService, vObj)
	newService.Spec.ClusterIP = ""
	newService.Spec.ClusterIPs = nil
	StripNodePorts(newService)
	return newService
}

func RewriteSelector(pObj, vObj *corev1.Service) {
	if vObj.Spec.Selector != nil {
		pObj.Spec.Selector = map[string]string{}
		for k, v := range vObj.Spec.Selector {
			pObj.Spec.Selector[translator.ConvertLabelKeyWithPrefix(translator.LabelPrefix, k)] = v
		}
		pObj.Spec.Selector[translate.NamespaceLabel] = vObj.Namespace
		pObj.Spec.Selector[translate.MarkerLabel] = translate.Suffix
	} else {
		pObj.Spec.Selector = nil
	}
}

func StripNodePorts(vObj *corev1.Service) {
	for i := range vObj.Spec.Ports {
		vObj.Spec.Ports[i].NodePort = 0
	}
}

func portsEqual(pObj, vObj *corev1.Service) bool {
	pSpec := pObj.Spec.DeepCopy()
	vSpec := vObj.Spec.DeepCopy()
	for i := range pSpec.Ports {
		pSpec.Ports[i].NodePort = 0
	}
	for i := range vSpec.Ports {
		vSpec.Ports[i].NodePort = 0
	}
	return equality.Semantic.DeepEqual(pSpec.Ports, vSpec.Ports)
}

func (s *serviceSyncer) translateUpdateBackwards(pObj, vObj *corev1.Service) *corev1.Service {
	var updated *corev1.Service

	if vObj.Spec.ClusterIP != pObj.Spec.ClusterIP {
		updated = newIfNil(updated, vObj)
		updated.Spec.ClusterIP = pObj.Spec.ClusterIP
	}

	if !equality.Semantic.DeepEqual(vObj.Spec.ExternalIPs, pObj.Spec.ExternalIPs) {
		updated = newIfNil(updated, vObj)
		updated.Spec.ExternalIPs = pObj.Spec.ExternalIPs
	}

	if vObj.Spec.LoadBalancerIP != pObj.Spec.LoadBalancerIP {
		updated = newIfNil(updated, vObj)
		updated.Spec.LoadBalancerIP = pObj.Spec.LoadBalancerIP
	}

	// check if we need to sync node ports from host to virtual
	if pObj.Spec.Type == vObj.Spec.Type && portsEqual(pObj, vObj) && !equality.Semantic.DeepEqual(vObj.Spec.Ports, pObj.Spec.Ports) {
		updated = newIfNil(updated, vObj)
		updated.Spec.Ports = pObj.Spec.Ports
	}

	return updated
}

func (s *serviceSyncer) translateUpdate(pObj, vObj *corev1.Service) *corev1.Service {
	var updated *corev1.Service

	// check annotations
	_, updatedAnnotations, updatedLabels := s.TranslateMetadataUpdate(vObj, pObj)
	// remove the ServiceBlockDeletion annotation if it's not needed
	if vObj.Spec.ClusterIP == pObj.Spec.ClusterIP {
		delete(updatedAnnotations, ServiceBlockDeletion)
	}
	if !equality.Semantic.DeepEqual(updatedAnnotations, pObj.Annotations) || !equality.Semantic.DeepEqual(updatedLabels, pObj.Labels) {
		updated = newIfNil(updated, pObj)
		updated.Annotations = updatedAnnotations
		updated.Labels = updatedLabels
	}

	// check ports
	if !equality.Semantic.DeepEqual(vObj.Spec.Ports, pObj.Spec.Ports) {
		updated = newIfNil(updated, pObj)
		updated.Spec.Ports = vObj.Spec.Ports

		// make sure node ports will be reset here
		StripNodePorts(updated)
	}

	// publish not ready addresses
	if vObj.Spec.PublishNotReadyAddresses != pObj.Spec.PublishNotReadyAddresses {
		updated = newIfNil(updated, pObj)
		updated.Spec.PublishNotReadyAddresses = vObj.Spec.PublishNotReadyAddresses
	}

	// type
	if vObj.Spec.Type != pObj.Spec.Type {
		updated = newIfNil(updated, pObj)
		updated.Spec.Type = vObj.Spec.Type
	}

	// external name
	if vObj.Spec.ExternalName != pObj.Spec.ExternalName {
		updated = newIfNil(updated, pObj)
		updated.Spec.ExternalName = vObj.Spec.ExternalName
	}

	// externalTrafficPolicy
	if vObj.Spec.ExternalTrafficPolicy != pObj.Spec.ExternalTrafficPolicy {
		updated = newIfNil(updated, pObj)
		updated.Spec.ExternalTrafficPolicy = vObj.Spec.ExternalTrafficPolicy
	}

	// session affinity
	if vObj.Spec.SessionAffinity != pObj.Spec.SessionAffinity {
		updated = newIfNil(updated, pObj)
		updated.Spec.SessionAffinity = vObj.Spec.SessionAffinity
	}

	// sessionAffinityConfig
	if !equality.Semantic.DeepEqual(vObj.Spec.SessionAffinityConfig, pObj.Spec.SessionAffinityConfig) {
		updated = newIfNil(updated, pObj)
		updated.Spec.SessionAffinityConfig = vObj.Spec.SessionAffinityConfig
	}

	// load balancer source ranges
	if !equality.Semantic.DeepEqual(vObj.Spec.LoadBalancerSourceRanges, pObj.Spec.LoadBalancerSourceRanges) {
		updated = newIfNil(updated, pObj)
		updated.Spec.LoadBalancerSourceRanges = vObj.Spec.LoadBalancerSourceRanges
	}

	// healthCheckNodePort
	if vObj.Spec.HealthCheckNodePort != pObj.Spec.HealthCheckNodePort {
		updated = newIfNil(updated, pObj)
		updated.Spec.HealthCheckNodePort = vObj.Spec.HealthCheckNodePort
	}

	// translate selector
	translated := pObj.DeepCopy()
	RewriteSelector(translated, vObj)
	if !equality.Semantic.DeepEqual(translated.Spec.Selector, pObj.Spec.Selector) {
		updated = newIfNil(updated, pObj)
		updated.Spec.Selector = translated.Spec.Selector
	}

	return updated
}

func newIfNil(updated *corev1.Service, pObj *corev1.Service) *corev1.Service {
	if updated == nil {
		return pObj.DeepCopy()
	}
	return updated
}
