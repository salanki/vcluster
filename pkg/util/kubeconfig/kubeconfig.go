package kubeconfig

import (
	"context"
	"fmt"
	"github.com/loft-sh/vcluster/pkg/util/translate"
	"github.com/pkg/errors"
	"io/ioutil"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/tools/clientcmd"
	"k8s.io/client-go/tools/clientcmd/api"
	"k8s.io/klog/v2"
	"os"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
)

const (
	DefaultSecretPrefix     = "vc-"
	KubeconfigSecretKey     = "config"
	CADataSecretKey         = "certificate-authority"
	CertificateSecretKey    = "client-certificate"
	CertificateKeySecretKey = "client-key"
)

func WriteKubeConfig(ctx context.Context, currentNamespaceClient client.Client, secretName, secretNamespace string, config *api.Config) error {
	out, err := clientcmd.Write(*config)
	if err != nil {
		return err
	}

	// try to write the kubeconfig file for backwards compatibility with older vcluster cli versions
	// intentionally ignore errors as this may fail for non root user, or if securityContext.readOnlyRootFilesystem = true
	err = os.MkdirAll("/root/.kube", 0755)
	if err == nil {
		err = ioutil.WriteFile("/root/.kube/config", out, 0666)
		if err != nil {
			klog.Infof("Failed writing /root/.kube/config file: %v ; This error might be expected if you are running as non-root user or securityContext.readOnlyRootFilesystem = true", err)
		}
	} else {
		klog.Infof("Failed to create /root/.kube folder for writing kube config: %v ; This error might be expected if you are running as non-root user or securityContext.readOnlyRootFilesystem = true", err)
	}

	if secretName != "" {
		clientCmdConfig := clientcmd.NewDefaultClientConfig(*config, &clientcmd.ConfigOverrides{CurrentContext: config.CurrentContext})
		clientConfig, err := clientCmdConfig.ClientConfig()
		if err != nil {
			return err
		}

		caData := clientConfig.CAData
		cert := clientConfig.CertData
		key := clientConfig.KeyData

		kubeConfigSecret := &corev1.Secret{
			ObjectMeta: metav1.ObjectMeta{
				Name:      secretName,
				Namespace: secretNamespace,
			},
		}
		result, err := controllerutil.CreateOrPatch(ctx, currentNamespaceClient, kubeConfigSecret, func() error {
			kubeConfigSecret.Type = corev1.SecretTypeOpaque
			if kubeConfigSecret.Data == nil {
				kubeConfigSecret.Data = map[string][]byte{}
			}
			kubeConfigSecret.Data[KubeconfigSecretKey] = out
			kubeConfigSecret.Data[CADataSecretKey] = caData
			kubeConfigSecret.Data[CertificateSecretKey] = cert
			kubeConfigSecret.Data[CertificateKeySecretKey] = key

			// set owner reference
			if translate.Owner != nil && translate.Owner.GetNamespace() == kubeConfigSecret.Namespace {
				kubeConfigSecret.OwnerReferences = translate.GetOwnerReference(nil)
			}
			return nil
		})
		if err != nil {
			return errors.Wrap(err, "apply generated kube config secret")
		} else if result != controllerutil.OperationResultNone {
			klog.Infof("Applied kube config secret %s/%s", kubeConfigSecret.Namespace, kubeConfigSecret.Name)
		}
	}

	return nil
}

func ReadKubeConfig(ctx context.Context, client *kubernetes.Clientset, suffix, namespace string) (*api.Config, error) {
	secret, err := client.CoreV1().Secrets(namespace).Get(ctx, GetDefaultSecretName(suffix), metav1.GetOptions{})
	if err != nil {
		return nil, fmt.Errorf("could not Get the %s secret in order to read kubeconfig: %v", GetDefaultSecretName(suffix), err)
	}
	config, found := secret.Data[KubeconfigSecretKey]
	if !found {
		return nil, fmt.Errorf("could not find the kube config (%s key) in the %s secret", KubeconfigSecretKey, GetDefaultSecretName(suffix))
	}
	return clientcmd.Load(config)
}

func GetDefaultSecretName(suffix string) string {
	return DefaultSecretPrefix + suffix
}
