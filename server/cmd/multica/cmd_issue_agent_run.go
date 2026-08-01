package main

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/url"
	"os"

	"github.com/spf13/cobra"

	"github.com/multica-ai/multica/server/internal/agentrun"
	"github.com/multica-ai/multica/server/internal/cli"
)

// Agent-run commands are intentionally JSON-first. The contract is a machine
// control-plane document, not a human comment or free-form metadata payload.
var issueAgentRunCmd = &cobra.Command{
	Use:   "agent-run",
	Short: "Manage machine-validated agent-run/v1 contracts",
}

var issueAgentRunCreateCmd = &cobra.Command{
	Use:   "create <issue-id>",
	Short: "Create a draft agent-run/v1 contract",
	Args:  exactArgs(1),
	RunE:  runIssueAgentRunCreate,
}

var issueAgentRunGetCmd = &cobra.Command{
	Use:   "get <issue-id> <run-id>",
	Short: "Get an agent-run/v1 contract",
	Args:  exactArgs(2),
	RunE:  runIssueAgentRunGet,
}

var issueAgentRunListCmd = &cobra.Command{
	Use:   "list <issue-id>",
	Short: "List agent-run/v1 contracts for an issue",
	Args:  exactArgs(1),
	RunE:  runIssueAgentRunList,
}

var issueAgentRunUpdateCmd = &cobra.Command{
	Use:   "update <issue-id>",
	Short: "Atomically update an agent-run/v1 contract",
	Args:  exactArgs(1),
	RunE:  runIssueAgentRunUpdate,
}

func init() {
	issueAgentRunCmd.AddCommand(issueAgentRunCreateCmd)
	issueAgentRunCmd.AddCommand(issueAgentRunGetCmd)
	issueAgentRunCmd.AddCommand(issueAgentRunListCmd)
	issueAgentRunCmd.AddCommand(issueAgentRunUpdateCmd)
	issueCmd.AddCommand(issueAgentRunCmd)

	addAgentRunContractFlags(issueAgentRunCreateCmd)
	issueAgentRunCreateCmd.Flags().String(
		"issue-status-mode",
		"follow_run",
		"Reconcile issue status from run status: follow_run or none",
	)

	addAgentRunContractFlags(issueAgentRunUpdateCmd)
	issueAgentRunUpdateCmd.Flags().Int(
		"expected-revision",
		0,
		"Current server revision; required for optimistic concurrency",
	)
}

func addAgentRunContractFlags(cmd *cobra.Command) {
	cmd.Flags().String("contract", "", "Inline agent-run/v1 JSON contract")
	cmd.Flags().Bool("contract-stdin", false, "Read the agent-run/v1 JSON contract from stdin")
	cmd.Flags().String("contract-file", "", "Read the agent-run/v1 JSON contract from a UTF-8 file")
}

func readAgentRunContract(cmd *cobra.Command) (agentrun.Contract, error) {
	raw, present, err := resolveTextFlag(cmd, "contract")
	if err != nil {
		return agentrun.Contract{}, err
	}
	if !present {
		return agentrun.Contract{}, fmt.Errorf("one of --contract, --contract-stdin, or --contract-file is required")
	}

	var contract agentrun.Contract
	decoder := json.NewDecoder(bytes.NewBufferString(raw))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&contract); err != nil {
		return agentrun.Contract{}, fmt.Errorf("decode agent-run/v1 contract: %w", err)
	}
	if err := ensureAgentRunJSONEOF(decoder); err != nil {
		return agentrun.Contract{}, err
	}
	if err := agentrun.Validate(contract); err != nil {
		return agentrun.Contract{}, err
	}
	return contract, nil
}

func ensureAgentRunJSONEOF(decoder *json.Decoder) error {
	var trailing any
	if err := decoder.Decode(&trailing); err != io.EOF {
		if err == nil {
			return fmt.Errorf("decode agent-run/v1 contract: multiple JSON values are not allowed")
		}
		return fmt.Errorf("decode agent-run/v1 contract: %w", err)
	}
	return nil
}

func runIssueAgentRunCreate(cmd *cobra.Command, args []string) error {
	contract, err := readAgentRunContract(cmd)
	if err != nil {
		return err
	}
	if contract.Status != "draft" {
		return fmt.Errorf("new agent run must start in draft")
	}
	issueStatusMode, _ := cmd.Flags().GetString("issue-status-mode")
	if issueStatusMode != "follow_run" && issueStatusMode != "none" {
		return fmt.Errorf("--issue-status-mode must be follow_run or none")
	}

	client, ctx, cancel, issueID, err := prepareAgentRunRequest(cmd, args[0])
	if err != nil {
		return err
	}
	defer cancel()

	body := map[string]any{
		"contract":          contract,
		"issue_status_mode": issueStatusMode,
	}
	var result map[string]any
	if err := client.PostJSON(ctx, "/api/issues/"+url.PathEscape(issueID)+"/agent-runs", body, &result); err != nil {
		return fmt.Errorf("create agent run: %w", err)
	}
	return cli.PrintJSON(os.Stdout, result)
}

func runIssueAgentRunGet(cmd *cobra.Command, args []string) error {
	client, ctx, cancel, issueID, err := prepareAgentRunRequest(cmd, args[0])
	if err != nil {
		return err
	}
	defer cancel()

	var result map[string]any
	path := "/api/issues/" + url.PathEscape(issueID) + "/agent-runs/" + url.PathEscape(args[1])
	if err := client.GetJSON(ctx, path, &result); err != nil {
		return fmt.Errorf("get agent run: %w", err)
	}
	return cli.PrintJSON(os.Stdout, result)
}

func runIssueAgentRunList(cmd *cobra.Command, args []string) error {
	client, ctx, cancel, issueID, err := prepareAgentRunRequest(cmd, args[0])
	if err != nil {
		return err
	}
	defer cancel()

	var result []map[string]any
	if err := client.GetJSON(ctx, "/api/issues/"+url.PathEscape(issueID)+"/agent-runs", &result); err != nil {
		return fmt.Errorf("list agent runs: %w", err)
	}
	return cli.PrintJSON(os.Stdout, result)
}

func runIssueAgentRunUpdate(cmd *cobra.Command, args []string) error {
	expectedRevision, _ := cmd.Flags().GetInt("expected-revision")
	if expectedRevision <= 0 {
		return fmt.Errorf("--expected-revision must be greater than zero")
	}
	contract, err := readAgentRunContract(cmd)
	if err != nil {
		return err
	}

	client, ctx, cancel, issueID, err := prepareAgentRunRequest(cmd, args[0])
	if err != nil {
		return err
	}
	defer cancel()

	body := map[string]any{
		"expected_revision": expectedRevision,
		"contract":          contract,
	}
	var result map[string]any
	path := "/api/issues/" + url.PathEscape(issueID) + "/agent-runs/" + url.PathEscape(contract.RunID)
	if err := client.PutJSON(ctx, path, body, &result); err != nil {
		return fmt.Errorf("update agent run: %w", err)
	}
	return cli.PrintJSON(os.Stdout, result)
}

func prepareAgentRunRequest(cmd *cobra.Command, issueRef string) (*cli.APIClient, context.Context, context.CancelFunc, string, error) {
	client, err := newAPIClient(cmd)
	if err != nil {
		return nil, nil, nil, "", err
	}
	ctx, cancel := cli.APIContext(context.Background())
	issue, err := resolveIssueRef(ctx, client, issueRef)
	if err != nil {
		cancel()
		return nil, nil, nil, "", fmt.Errorf("resolve issue: %w", err)
	}
	return client, ctx, cancel, issue.ID, nil
}
