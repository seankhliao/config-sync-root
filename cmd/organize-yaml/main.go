package main

import (
	"flag"
	"fmt"
	"io/fs"
	"log/slog"
	"os"
	"path/filepath"
	"slices"
	"strings"

	"sigs.k8s.io/kustomize/kyaml/kio"
	"sigs.k8s.io/kustomize/kyaml/yaml"
)

var (
	// kubectl api-resources -o wide | rg false
	ClusterKinds = []string{
		"APIService",
		"CertificateSigningRequest",
		"ClusterInterceptor",
		"ClusterRole",
		"ClusterRoleBinding",
		"ClusterSelector",
		"ClusterTask",
		"ClusterTriggerBinding",
		"ComponentStatus",
		"CSIDriver",
		"CSINode",
		"CustomResourceDefinition",
		"FlowSchema",
		"GatewayClass",
		"IngressClass",
		"MutatingWebhookConfiguration",
		// "Namespace",
		"NamespaceSelector",
		"Node",
		"NodeMetrics",
		"PersistentVolume",
		"PriorityClass",
		"PriorityLevelConfiguration",
		"RuntimeClass",
		"SelfSubjectAccessReview",
		"SelfSubjectReview",
		"SelfSubjectRulesReview",
		"StorageClass",
		"SubjectAccessReview",
		"TokenReview",
		"ValidatingWebhookConfiguration",
		"VolumeAttachment",
	}
	NameSanitizer = strings.NewReplacer("/", "_", ".", "_", ":", "_")
)

func main() {
	var inputDir, outputDir string
	flag.StringVar(&inputDir, "input", "_input", "directory of input files")
	flag.StringVar(&outputDir, "output", "output", "directory to output files")
	flag.Parse()

	outputs, err := processInput(inputDir)
	if err != nil {
		slog.Error(err.Error())
		os.Exit(1)
	}

	err = writeOutputs(outputDir, outputs)
	if err != nil {
		slog.Error(err.Error())
		os.Exit(1)
	}
}

func processInput(inputDir string) (map[string][]*yaml.RNode, error) {
	outputs := make(map[string][]*yaml.RNode)
	fsys := os.DirFS(inputDir)
	err := fs.WalkDir(fsys, ".", func(path string, d fs.DirEntry, err error) error {
		if d.IsDir() || err != nil || filepath.Ext(path) != ".yaml" {
			return err
		}
		b, err := fs.ReadFile(fsys, path)
		if err != nil {
			return fmt.Errorf("read %s: %w", path, err)
		}
		nodes, err := kio.ParseAll(string(b))
		if err != nil {
			return fmt.Errorf("parse %s: %w", path, err)
		}

		for _, node := range nodes {
			node := node
			meta, err := node.GetMeta()
			if err != nil {
				return fmt.Errorf("parse meta in %s: %w", path, err)
			}
			outPath := meta.Namespace
			if strings.HasPrefix(meta.Kind, "Cluster") || slices.Contains(ClusterKinds, meta.Kind) {
				outPath = "cluster-scope"
			} else if meta.Kind == "Namespace" {
				outPath = meta.Name
			} else if outPath == "" {
				outPath = "default"
			}
			fullKind := strings.ToLower(meta.Kind)
			apiGroup, _, ok := strings.Cut(meta.APIVersion, "/")
			if ok {
				fullKind = fullKind + "." + apiGroup
			}

			fileName := strings.ToLower(NameSanitizer.Replace(meta.Name)) + "." + fullKind + ".yaml"
			outPath = filepath.Join(outPath, fileName)

			outputs[outPath] = append(outputs[outPath], node)
		}

		return nil
	})
	return outputs, err
}

func writeOutputs(baseDir string, outputs map[string][]*yaml.RNode) error {
	for name, nodes := range outputs {
		fullPath := filepath.Join(baseDir, name)
		dir := filepath.Dir(fullPath)
		err := os.MkdirAll(dir, 0o755)
		if err != nil {
			return fmt.Errorf("create directory %s: %w", dir, err)
		}
		b, err := kio.StringAll(nodes)
		if err != nil {
			return fmt.Errorf("stringify %s: %w", name, err)
		}
		err = os.WriteFile(fullPath, []byte(b), 0o644)
		if err != nil {
			return fmt.Errorf("write %s: %w", fullPath, err)
		}
	}
	return nil
}
