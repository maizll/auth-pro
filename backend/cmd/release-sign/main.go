// release-sign 是打包和发布时用的签名工具，不进发布包。
//
//	release-sign manifest <打包目录> <版本号>   写 manifest.json；环境变量 AUTH_PRO_UPDATE_SIGNING_KEY 有私钥就签名
//	release-sign verify <安装包.tar.gz> <版本号>  用程序内置公钥核对安装包，和在线更新走同一段校验
//
// AUTH_PRO_REQUIRE_UPDATE_SIGNATURE=1 时没有私钥直接失败，发布工作流靠它保证不会发出没签名的包。
package main

import (
	"crypto/ed25519"
	"fmt"
	"os"
	"strings"

	"auto_pro/updatesign"
)

func main() {
	if err := run(os.Args[1:]); err != nil {
		fmt.Fprintln(os.Stderr, "release-sign:", err)
		os.Exit(1)
	}
}

func run(args []string) error {
	if len(args) != 3 {
		return fmt.Errorf("用法：release-sign manifest <目录> <版本号> | release-sign verify <安装包> <版本号>")
	}
	pub, err := updatesign.ParsePublicKey(updatesign.PublicKey)
	if err != nil {
		return err
	}
	switch args[0] {
	case "manifest":
		var key ed25519.PrivateKey
		if text := strings.TrimSpace(os.Getenv("AUTH_PRO_UPDATE_SIGNING_KEY")); text != "" {
			if key, err = updatesign.ParsePrivateKey(text); err != nil {
				return err
			}
			if !pub.Equal(key.Public()) {
				return fmt.Errorf("发布私钥和程序内置公钥不是一对，签出来的包客户站会拒绝安装")
			}
		} else if os.Getenv("AUTH_PRO_REQUIRE_UPDATE_SIGNATURE") == "1" {
			return fmt.Errorf("没有配置发布私钥 AUTH_PRO_UPDATE_SIGNING_KEY，不能发布没签名的更新包")
		}
		if err := updatesign.WriteManifest(args[1], args[2], key); err != nil {
			return err
		}
		if key == nil {
			fmt.Println("manifest.json 已写入（未签名，只能手动安装，1.8.6 起的站点在线更新会拒绝）")
		} else {
			fmt.Println("manifest.json 已写入并签名")
		}
		return nil
	case "verify":
		if err := updatesign.VerifyPackage(args[1], args[2], pub); err != nil {
			return err
		}
		fmt.Println("签名校验通过")
		return nil
	}
	return fmt.Errorf("不认识的子命令 %s", args[0])
}
