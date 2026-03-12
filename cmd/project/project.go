package project

import (
	"fmt"

	"github.com/spf13/cobra"

	"github.com/team-xquare/xquare-cli/internal/api"
	"github.com/team-xquare/xquare-cli/internal/output"
)

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
		newAddMemberCmd(),
		newRemoveMemberCmd(),
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
			rows := make([][]string, len(projects))
			for i, p := range projects {
				rows[i] = []string{p}
			}
			output.Table([]string{"NAME"}, rows)
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
			return output.JSON(p)
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
			if dryRun {
				output.Info(fmt.Sprintf("[dry-run] would delete project: %s", args[0]))
				return nil
			}
			if !yes {
				return fmt.Errorf("use --yes to confirm deletion (this will delete the project and all its resources)")
			}
			c := api.FromCmd(cmd)
			if err := c.DeleteProject(cmd.Context(), args[0]); err != nil {
				return err
			}
			output.Success("deleted project " + args[0])
			return nil
		},
	}
	cmd.Flags().BoolVar(&yes, "yes", false, "confirm deletion")
	cmd.Flags().BoolVar(&dryRun, "dry-run", false, "show what would happen")
	return cmd
}

func newMembersCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "members",
		Short: "List project members",
		RunE: func(cmd *cobra.Command, args []string) error {
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
				return output.JSON(members)
			}
			if len(members) == 0 {
				output.Info("no members found")
				return nil
			}
			rows := make([][]string, len(members))
			for i, m := range members {
				rows[i] = []string{m}
			}
			output.Table([]string{"USERNAME"}, rows)
			return nil
		},
	}
}

func newAddMemberCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "add-member <github-username>",
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

func newRemoveMemberCmd() *cobra.Command {
	var yes bool
	cmd := &cobra.Command{
		Use:   "remove-member <github-username>",
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
