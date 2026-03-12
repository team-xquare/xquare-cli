package server

import (
	"fmt"
	"os"

	"github.com/spf13/cobra"

	"github.com/team-xquare/xquare-cli/internal/api"
	"github.com/team-xquare/xquare-cli/internal/output"
)

func coalesce(vals ...any) any {
	for _, v := range vals {
		if v != nil {
			return v
		}
	}
	return nil
}

func NewServerCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "server",
		Short: "Server administration commands (admin only)",
	}
	cmd.AddCommand(newAllowlistCmd())
	return cmd
}

func newAllowlistCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "allowlist",
		Short: "Manage the user allowlist",
	}
	cmd.AddCommand(
		newAllowlistListCmd(),
		newAllowlistAddCmd(),
		newAllowlistRemoveCmd(),
	)
	return cmd
}

func newAllowlistListCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "list",
		Short: "List allowed users",
		RunE: func(cmd *cobra.Command, args []string) error {
			c := api.FromCmd(cmd)
			users, err := c.ListAllowlist(cmd.Context())
			if err != nil {
				return err
			}
			if api.IsJSON(cmd) {
				return output.JSON(users)
			}
			if len(users) == 0 {
				output.Info("allowlist is empty — all authenticated users have access")
				return nil
			}
			rows := make([][]string, 0, len(users))
			for _, u := range users {
				// handle both lowercase (json tagged) and capitalized (untagged) field names
				idVal := coalesce(u["id"], u["ID"])
				var id string
				switch v := idVal.(type) {
				case float64:
					id = fmt.Sprintf("%d", int64(v))
				default:
					id = fmt.Sprintf("%v", v)
				}
				username := fmt.Sprintf("%v", coalesce(u["username"], u["Username"]))
				rows = append(rows, []string{username, id})
			}
			output.Table([]string{"Username", "GitHub ID"}, rows)
			return nil
		},
	}
}

func newAllowlistAddCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "add <github-username>",
		Short: "Add a user to the allowlist",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			c := api.FromCmd(cmd)
			result, err := c.AddAllowlist(cmd.Context(), args[0])
			if err != nil {
				return err
			}
			if api.IsJSON(cmd) {
				return output.JSON(result)
			}
			output.Success(fmt.Sprintf("added %s (id=%v) to allowlist", result["username"], result["id"]))
			return nil
		},
	}
}

func newAllowlistRemoveCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "remove <github-username>",
		Short: "Remove a user from the allowlist",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			username := args[0]
			c := api.FromCmd(cmd)
			if err := c.RemoveAllowlist(cmd.Context(), username); err != nil {
				return err
			}
			if api.IsJSON(cmd) {
				return output.JSON(map[string]string{"username": username, "status": "removed"})
			}
			output.Success(fmt.Sprintf("removed %s from allowlist", username))
			os.Stderr.WriteString("")
			return nil
		},
	}
}
