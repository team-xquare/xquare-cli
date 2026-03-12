package addon

import (
	"fmt"

	"github.com/spf13/cobra"

	"github.com/team-xquare/xquare-cli/internal/api"
	"github.com/team-xquare/xquare-cli/internal/output"
)

func NewAddonCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "addon",
		Short: "Manage addons (databases, caches, etc.)",
	}
	cmd.AddCommand(
		newAddonListCmd(),
		newAddonCreateCmd(),
		newAddonDeleteCmd(),
		newAddonConnectionCmd(),
	)
	return cmd
}

func newAddonListCmd() *cobra.Command {
	return &cobra.Command{
		Use:     "list",
		Short:   "List addons in a project",
		Aliases: []string{"ls"},
		RunE: func(cmd *cobra.Command, args []string) error {
			c := api.FromCmd(cmd)
			project, err := api.RequireProject(cmd)
			if err != nil {
				return err
			}
			addons, err := c.ListAddons(cmd.Context(), project)
			if err != nil {
				return err
			}
			if api.IsJSON(cmd) {
				return output.JSON(addons)
			}
			if len(addons) == 0 {
				output.Info("no addons found")
				return nil
			}
			rows := make([][]string, 0, len(addons))
			for _, a := range addons {
				readyStr := "⏳ 프로비저닝 중"
				if fmt.Sprintf("%v", a["ready"]) == "true" {
					readyStr = "✓ 사용 가능"
				}
				rows = append(rows, []string{
					fmt.Sprintf("%v", a["name"]),
					fmt.Sprintf("%v", a["type"]),
					fmt.Sprintf("%v", a["storage"]),
					readyStr,
				})
			}
			output.Table([]string{"NAME", "TYPE", "STORAGE", "STATUS"}, rows)
			return nil
		},
	}
}

func newAddonCreateCmd() *cobra.Command {
	var storage string
	var bootstrap string
	var dryRun bool

	cmd := &cobra.Command{
		Use:   "create <name> <type>",
		Short: "Create an addon (mysql, postgresql, redis, mongodb, etc.)",
		Args:  cobra.ExactArgs(2),
		RunE: func(cmd *cobra.Command, args []string) error {
			project, err := api.RequireProject(cmd)
			if err != nil {
				return err
			}
			if dryRun {
				output.Info(fmt.Sprintf("[dry-run] would create %s addon '%s' in project %s", args[1], args[0], project))
				return nil
			}
			c := api.FromCmd(cmd)
			body := map[string]string{
				"name":    args[0],
				"type":    args[1],
				"storage": storage,
			}
			if bootstrap != "" {
				body["bootstrap"] = bootstrap
			}
			result, err := c.CreateAddon(cmd.Context(), project, body)
			if err != nil {
				return err
			}
			if api.IsJSON(cmd) {
				return output.JSON(result)
			}
			output.Success(fmt.Sprintf("created addon '%s' (%s)", args[0], args[1]))
			output.Info("DB 프로비저닝 중... (약 1~2분 소요)")
			output.Info(fmt.Sprintf("  xquare addon list   # 준비 상태 확인"))
			return nil
		},
	}
	cmd.Flags().StringVar(&storage, "storage", "10Gi", "storage size (e.g. 10Gi)")
	cmd.Flags().StringVar(&bootstrap, "bootstrap", "", "bootstrap SQL/script")
	cmd.Flags().BoolVar(&dryRun, "dry-run", false, "show what would happen")
	return cmd
}

func newAddonDeleteCmd() *cobra.Command {
	var yes, dryRun bool
	cmd := &cobra.Command{
		Use:   "delete <name>",
		Short: "Delete an addon",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			project, err := api.RequireProject(cmd)
			if err != nil {
				return err
			}
			if dryRun {
				output.Info(fmt.Sprintf("[dry-run] would delete addon '%s' from project %s", args[0], project))
				return nil
			}
			if !yes {
				return fmt.Errorf("use --yes to confirm deletion")
			}
			c := api.FromCmd(cmd)
			if err := c.DeleteAddon(cmd.Context(), project, args[0]); err != nil {
				return err
			}
			output.Success(fmt.Sprintf("deleted addon '%s'", args[0]))
			return nil
		},
	}
	cmd.Flags().BoolVar(&yes, "yes", false, "confirm deletion")
	cmd.Flags().BoolVar(&dryRun, "dry-run", false, "show what would happen")
	return cmd
}

func newAddonConnectionCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "connection <name>",
		Short: "Show connection info for an addon",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			c := api.FromCmd(cmd)
			project, err := api.RequireProject(cmd)
			if err != nil {
				return err
			}
			conn, err := c.GetAddonConnection(cmd.Context(), project, args[0])
			if err != nil {
				return err
			}
			if api.IsJSON(cmd) {
				return output.JSON(conn)
			}
			readyStr := "⏳ 프로비저닝 중 (아직 접속 불가)"
			if fmt.Sprintf("%v", conn["ready"]) == "true" {
				readyStr = "✓ 사용 가능"
			}
			rows := [][]string{
				{"Status", readyStr},
				{"Type", fmt.Sprintf("%v", conn["type"])},
				{"Host", fmt.Sprintf("%v", conn["host"])},
				{"Port", fmt.Sprintf("%v", conn["port"])},
				{"Password", fmt.Sprintf("%v", conn["password"])},
			}
			output.Table([]string{"FIELD", "VALUE"}, rows)
			return nil
		},
	}
}
