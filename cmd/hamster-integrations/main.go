package main

import (
	"context"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"time"

	"github.com/hamster-switch/server-integrations/internal/release"
	"github.com/hamster-switch/server-integrations/internal/target"
	"github.com/hamster-switch/server-integrations/internal/updater"
)

const version = "0.3.0"

type options struct {
	target   string
	mode     string
	tag      string
	stateDir string
	yes      bool
}

func main() {
	if err := run(os.Args[1:]); err != nil {
		fmt.Fprintln(os.Stderr, "错误:", err)
		os.Exit(1)
	}
}

func run(args []string) error {
	if len(args) == 0 {
		return usageError()
	}
	if args[0] == "version" || args[0] == "--version" {
		fmt.Println(version)
		return nil
	}
	command := args[0]
	if command != "inspect" && command != "check" && command != "update" && command != "rollback" {
		return usageError()
	}
	if len(args) < 2 {
		return errors.New("缺少组件名称，只允许 sub2api 或 new-api")
	}
	component := args[1]
	if component != "sub2api" && component != "new-api" {
		return fmt.Errorf("不支持的组件 %q", component)
	}
	flags := flag.NewFlagSet(command, flag.ContinueOnError)
	flags.SetOutput(os.Stderr)
	defaults := "/var/lib/hamster-integrations"
	if value := os.Getenv("HAMSTER_INTEGRATIONS_STATE_DIR"); value != "" {
		defaults = value
	}
	var opts options
	flags.StringVar(&opts.target, "target", "", "上游源码绝对路径")
	flags.StringVar(&opts.mode, "mode", "manual", "manual、systemd 或 docker-compose")
	flags.StringVar(&opts.tag, "release", "", "固定正式 Release tag；默认 latest")
	flags.StringVar(&opts.stateDir, "state-dir", defaults, "CLI 本地状态目录")
	flags.BoolVar(&opts.yes, "yes", false, "在非交互环境确认已审阅差异")
	if err := flags.Parse(args[2:]); err != nil {
		return err
	}
	if len(flags.Args()) != 0 {
		return fmt.Errorf("未知参数: %s", strings.Join(flags.Args(), " "))
	}
	if command == "rollback" {
		if runtime.GOOS != "linux" {
			return errors.New("rollback is supported only on Linux production servers")
		}
		return rollback(component, opts)
	}
	if opts.target == "" {
		return errors.New("--target 必填")
	}
	root, err := target.Resolve(opts.target)
	if err != nil {
		return err
	}
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Minute)
	defer cancel()
	client := release.NewClient()
	verified, err := client.Fetch(ctx, opts.tag, component)
	if err != nil {
		return err
	}
	if err := release.CheckMinimumCLI(version, verified.Manifest.MinCLI); err != nil {
		return err
	}
	report := target.Inspect(root, verified.Manifest)
	if command == "inspect" {
		return printJSON(report)
	}
	fmt.Printf("组件: %s\n补丁版本: %s\nRelease: %s\n目标: %s\n兼容: %t\n", component, verified.Manifest.PatchVersion, verified.Tag, root, report.Compatible)
	for _, file := range report.Files {
		fmt.Printf("  - %s: %t\n", file.Path, file.Matches)
	}
	for _, message := range report.AnchorErrors {
		fmt.Printf("  - %s\n", message)
	}
	if !report.Compatible {
		return errors.New("上游版本或本地文件指纹不匹配；仅允许 inspect，不存在强制应用参数")
	}
	if command == "check" {
		fmt.Println("检查完成：未下载补丁包，未修改源码，未构建或重启。")
		return nil
	}
	if runtime.GOOS != "linux" {
		return errors.New("patch application is supported only on Linux production servers")
	}
	return update(ctx, client, verified, root, opts)
}

func update(ctx context.Context, client release.Client, verified release.Verified, root string, opts options) error {
	manifest := verified.Manifest
	fmt.Println("将执行以下受签名清单约束的操作:")
	for _, file := range manifest.Patch.Files {
		fmt.Printf("  修改源码: %s\n", file.Path)
	}
	for _, step := range manifest.Deployment.BuildSteps {
		fmt.Printf("  构建步骤: %s (%s)\n", step.Kind, step.Workdir)
	}
	if opts.mode == "systemd" && manifest.Deployment.Systemd != nil {
		fmt.Printf("  重启 systemd 服务: %s\n", manifest.Deployment.Systemd.Service)
	}
	if opts.mode == "docker-compose" && manifest.Deployment.DockerCompose != nil {
		fmt.Printf("  重建并启动 compose 服务: %s\n", strings.Join(manifest.Deployment.DockerCompose.Services, ", "))
	}
	if opts.mode == "manual" {
		fmt.Println("  manual 模式不会自动构建或重启。")
	}
	if !opts.yes {
		fmt.Print("确认应用此补丁？输入 apply 继续: ")
		var answer string
		if _, err := fmt.Scanln(&answer); err != nil || answer != "apply" {
			return errors.New("已取消，未修改任何文件")
		}
	}
	bundle, err := client.DownloadVerifiedAsset(ctx, verified, manifest.Patch.BundleAsset)
	if err != nil {
		return err
	}
	if err := updater.Apply(ctx, root, filepath.Clean(opts.stateDir), opts.mode, manifest, bundle); err != nil {
		return err
	}
	if opts.mode == "manual" {
		fmt.Println("源码补丁已应用；服务尚未更新，请完成以下人工步骤:")
		for _, step := range manifest.Deployment.ManualSteps {
			fmt.Printf("  - %s\n", step)
		}
	} else {
		fmt.Println("补丁、构建、重启和健康检查均已完成。")
	}
	return nil
}

func rollback(component string, opts options) error {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Minute)
	defer cancel()
	state, err := updater.LoadState(filepath.Clean(opts.stateDir), component)
	if err != nil {
		return err
	}
	fmt.Printf("将恢复 %s 的备份 %s\n", component, state.BackupDir)
	if !opts.yes {
		fmt.Print("确认回滚？输入 rollback 继续: ")
		var answer string
		if _, err := fmt.Scanln(&answer); err != nil || answer != "rollback" {
			return errors.New("已取消回滚")
		}
	}
	if err := updater.Rollback(ctx, filepath.Clean(opts.stateDir), component); err != nil {
		return err
	}
	fmt.Println("回滚完成。")
	return nil
}

func printJSON(value any) error {
	encoder := json.NewEncoder(os.Stdout)
	encoder.SetIndent("", "  ")
	return encoder.Encode(value)
}

func usageError() error {
	return errors.New("用法: hamster-integrations <inspect|check|update|rollback> <sub2api|new-api> --target <path>")
}
