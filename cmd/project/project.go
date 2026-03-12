package project

import (
	"fmt"
	"regexp"

	"github.com/spf13/cobra"

	"github.com/team-xquare/xquare-cli/internal/api"
	"github.com/team-xquare/xquare-cli/internal/output"
)

var projectNameRe = regexp.MustCompile(`^[a-z0-9]{2,63}$`)

func validateProjectName(name string) error {
	if !projectNameRe.MatchString(name) {
		return fmt.Errorf("invalid project name %q: lowercase letters and numbers only, no hyphens (2-63 chars)", name)
	}
	return nil
}

func NewProjectCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:     "project",
		Short:   "Manage projects",
		Aliases: []string{"proj", "p"},
	}
	cmd.AddCommand(
		newListCmd(),
		newGetCmd(),
		newCreateCmd(),
		newDeleteCmd(),
		newMembersCmd(),
	)
	return cmd
}

func newListCmd() *cobra.Command {
	return &cobra.Command{
		Use:     "list",
		Short:   "List all projects",
		Aliases: []string{"ls"},
		RunE: func(cmd *cobra.Command, args []string) error {
			c := api.FromCmd(cmd)
			projects, err := c.ListProjects(cmd.Context())
			if err != nil {
				return err
			}
			if api.IsJSON(cmd) {
				return output.JSON(projects)
			}
			if len(projects) == 0 {
				output.Info("no projects found")
				return nil
			}
			// Load current linked project for highlighting
			current := ""
			if pc, _ := api.GetCurrentProject(cmd); pc != "" {
				current = pc
			}
			rows := make([][]string, len(projects))
			for i, p := range projects {
				name := p
				if p == current {
					name = "* " + p
				}
				rows[i] = []string{name}
			}
			output.Table([]string{"NAME"}, rows)
			if current != "" {
				found := false
				for _, p := range projects {
					if p == current {
						found = true
						break
					}
				}
				if found {
					output.Info(fmt.Sprintf("(* = linked project: %s)", current))
				} else {
					output.Warn(fmt.Sprintf("linked project %q not found or no access — run: xquare link <project>", current))
				}
			}
			return nil
		},
	}
}

func newGetCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "get <project>",
		Short: "Show project details",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			c := api.FromCmd(cmd)
			p, err := c.GetProject(cmd.Context(), args[0])
			if err != nil {
				return err
			}
			if api.IsJSON(cmd) {
				return output.JSON(p)
			}
			rows := [][]string{{"Name", args[0]}}
			if apps, ok := p["applications"].([]any); ok {
				rows = append(rows, []string{"Apps", fmt.Sprintf("%d", len(apps))})
				for _, a := range apps {
					if app, ok := a.(map[string]any); ok {
						rows = append(rows, []string{"  app", fmt.Sprintf("%v", app["name"])})
					}
				}
			}
			if addons, ok := p["addons"].([]any); ok && len(addons) > 0 {
				rows = append(rows, []string{"Addons", fmt.Sprintf("%d", len(addons))})
				for _, a := range addons {
					if addon, ok := a.(map[string]any); ok {
						rows = append(rows, []string{"  addon", fmt.Sprintf("%v (%v)", addon["name"], addon["type"])})
					}
				}
			}
			if owners, ok := p["owners"].([]any); ok {
				for _, o := range owners {
					if owner, ok := o.(map[string]any); ok {
						rows = append(rows, []string{"Member", fmt.Sprintf("%v", owner["username"])})
					}
				}
			}
			output.Table([]string{"FIELD", "VALUE"}, rows)
			return nil
		},
	}
}

func newCreateCmd() *cobra.Command {
	var dryRun bool
	cmd := &cobra.Command{
		Use:   "create <name>",
		Short: "Create a new project",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			if err := validateProjectName(args[0]); err != nil {
				return err
			}
			if dryRun {
				output.Info(fmt.Sprintf("[dry-run] would create project: %s", args[0]))
				return nil
			}
			c := api.FromCmd(cmd)
			if err := c.CreateProject(cmd.Context(), args[0]); err != nil {
				return err
			}
			output.Success("created project " + args[0])
			return nil
		},
	}
	cmd.Flags().BoolVar(&dryRun, "dry-run", false, "show what would happen without making changes")
	return cmd
}

func newDeleteCmd() *cobra.Command {
	var yes bool
	var dryRun bool
	cmd := &cobra.Command{
		Use:   "delete <project>",
		Short: "Delete a project",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			projectName := args[0]
			if dryRun {
				output.Info(fmt.Sprintf("[dry-run] would delete project: %s", projectName))
				return nil
			}
			c := api.FromCmd(cmd)
			if !yes {
				// Show what will be deleted
				if p, err := c.GetProject(cmd.Context(), projectName); err == nil {
					appCount := 0
					addonCount := 0
					if apps, ok := p["applications"].([]any); ok {
						appCount = len(apps)
					}
					if addons, ok := p["addons"].([]any); ok {
						addonCount = len(addons)
					}
					if appCount > 0 || addonCount > 0 {
						output.Info(fmt.Sprintf("project %q contains %d app(s) and %d addon(s) — all will be deleted", projectName, appCount, addonCount))
					}
				}
				return fmt.Errorf("use --yes to confirm deletion of project %q", projectName)
			}
			if err := c.DeleteProject(cmd.Context(), projectName); err != nil {
				return err
			}
			output.Success("deleted project " + projectName)
			return nil
		},
	}
	cmd.Flags().BoolVar(&yes, "yes", false, "confirm deletion")
	cmd.Flags().BoolVar(&dryRun, "dry-run", false, "show what would happen")
	return cmd
}

func newMembersCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "members",
		Short: "Manage project members",
		RunE: func(cmd *cobra.Command, args []string) error {
			// Default action: list members
			c := api.FromCmd(cmd)
			project, err := api.RequireProject(cmd)
			if err != nil {
				return err
			}
			members, err := c.ListMembers(cmd.Context(), project)
			if err != nil {
				return err
			}
			if api.IsJSON(cmd) {
				if members == nil {
					members = []api.Owner{}
				}
				return output.JSON(members)
			}
			if len(members) == 0 {
				output.Info("no members found")
				return nil
			}
			rows := make([][]string, len(members))
			for i, m := range members {
				rows[i] = []string{m.Username, fmt.Sprintf("%d", m.ID)}
			}
			output.Table([]string{"USERNAME", "GITHUB ID"}, rows)
			return nil
		},
	}
	cmd.AddCommand(newMembersAddCmd(), newMembersRemoveCmd())
	return cmd
}

func newMembersAddCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "add <github-username>",
		Short: "Add a member to the project",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			c := api.FromCmd(cmd)
			project, err := api.RequireProject(cmd)
			if err != nil {
				return err
			}
			if err := c.AddMember(cmd.Context(), project, args[0]); err != nil {
				return err
			}
			output.Success(fmt.Sprintf("added %s to project %s", args[0], project))
			return nil
		},
	}
}

func newMembersRemoveCmd() *cobra.Command {
	var yes bool
	cmd := &cobra.Command{
		Use:   "remove <github-username>",
		Short: "Remove a member from the project",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			if !yes {
				return fmt.Errorf("use --yes to confirm removing %s", args[0])
			}
			c := api.FromCmd(cmd)
			project, err := api.RequireProject(cmd)
			if err != nil {
				return err
			}
			if err := c.RemoveMember(cmd.Context(), project, args[0]); err != nil {
				return err
			}
			output.Success(fmt.Sprintf("removed %s from project %s", args[0], project))
			return nil
		},
	}
	cmd.Flags().BoolVar(&yes, "yes", false, "confirm removal")
	return cmd
}
