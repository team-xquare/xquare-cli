package server

import (
	"fmt"
	"sort"
	"strings"

	"github.com/spf13/cobra"

	"github.com/team-xquare/xquare-cli/internal/api"
	"github.com/team-xquare/xquare-cli/internal/output"
)

func newUsersCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "users",
		Short: "List or inspect platform users (admin only)",
	}
	cmd.AddCommand(
		newUsersListCmd(),
		newUsersGetCmd(),
	)
	return cmd
}

func newUsersListCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "list",
		Short: "List all platform users with allowlist status and project memberships",
		RunE: func(cmd *cobra.Command, args []string) error {
			c := api.FromCmd(cmd)
			users, err := c.ListUsers(cmd.Context())
			if err != nil {
				return err
			}
			if api.IsJSON(cmd) {
				return output.JSON(users)
			}
			if len(users) == 0 {
				output.Info("no users found")
				return nil
			}
			rows := make([][]string, 0, len(users))
			for _, u := range users {
				username := fmt.Sprintf("%v", coalesce(u["username"], u["Username"]))
				inAllowlist := fmt.Sprintf("%v", coalesce(u["inAllowlist"], u["InAllowlist"]))
				// projects is []any when decoded from JSON
				var projects string
				if ps, ok := u["projects"].([]any); ok {
					strs := make([]string, 0, len(ps))
					for _, p := range ps {
						strs = append(strs, fmt.Sprintf("%v", p))
					}
					sort.Strings(strs)
					projects = strings.Join(strs, ", ")
				}
				rows = append(rows, []string{username, inAllowlist, projects})
			}
			// Sort by username
			sort.Slice(rows, func(i, j int) bool { return rows[i][0] < rows[j][0] })
			output.Table([]string{"Username", "Allowlist", "Projects"}, rows)
			return nil
		},
	}
}

func newUsersGetCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "get <github-username>",
		Short: "Show a user's allowlist status and project memberships",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			c := api.FromCmd(cmd)
			u, err := c.GetUser(cmd.Context(), args[0])
			if err != nil {
				return err
			}
			if api.IsJSON(cmd) {
				return output.JSON(u)
			}
			inAllowlist := fmt.Sprintf("%v", coalesce(u["inAllowlist"], u["InAllowlist"]))
			idVal := coalesce(u["id"], u["ID"])
			var id string
			switch v := idVal.(type) {
			case float64:
				id = fmt.Sprintf("%d", int64(v))
			default:
				id = fmt.Sprintf("%v", v)
			}
			var projects string
			if ps, ok := u["projects"].([]any); ok {
				strs := make([]string, 0, len(ps))
				for _, p := range ps {
					strs = append(strs, fmt.Sprintf("%v", p))
				}
				sort.Strings(strs)
				projects = strings.Join(strs, ", ")
				if projects == "" {
					projects = "(none)"
				}
			}
			output.Table([]string{"Field", "Value"}, [][]string{
				{"Username", fmt.Sprintf("%v", coalesce(u["username"], u["Username"]))},
				{"GitHub ID", id},
				{"Allowlist", inAllowlist},
				{"Projects", projects},
			})
			return nil
		},
	}
}
