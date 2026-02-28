package cmd

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"strings"
	"text/tabwriter"
	"time"

	"github.com/spf13/cobra"
)

var (
	clipServer string
)

func clipAPIKey() string {
	if apiKey != "" {
		return apiKey
	}
	return os.Getenv("GOJOPLIN_API_KEY")
}

func clipBaseURL() string {
	if clipServer != "" {
		return strings.TrimRight(clipServer, "/")
	}
	return "http://localhost:41184"
}

var httpClient = &http.Client{Timeout: 30 * time.Second}

func clipRequest(method, path string, body interface{}) ([]byte, error) {
	key := clipAPIKey()
	if key == "" {
		return nil, fmt.Errorf("--api-key flag or GOJOPLIN_API_KEY env var is required")
	}

	var bodyReader io.Reader
	if body != nil {
		data, err := json.Marshal(body)
		if err != nil {
			return nil, err
		}
		bodyReader = bytes.NewReader(data)
	}

	req, err := http.NewRequest(method, clipBaseURL()+path, bodyReader)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Authorization", "Bearer "+key)
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}

	resp, err := httpClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	data, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}

	if resp.StatusCode >= 400 {
		return nil, fmt.Errorf("HTTP %d: %s", resp.StatusCode, string(data))
	}
	return data, nil
}

func prettyJSON(data []byte) string {
	var buf bytes.Buffer
	if err := json.Indent(&buf, data, "", "  "); err != nil {
		return string(data)
	}
	return buf.String()
}

// clipCmd is the parent command for clipper API interactions.
var clipCmd = &cobra.Command{
	Use:   "clip",
	Short: "Interact with the Joplingo clipper API",
}

// --- ping ---

var clipPingCmd = &cobra.Command{
	Use:   "ping",
	Short: "Ping the clipper server",
	RunE: func(cmd *cobra.Command, args []string) error {
		data, err := clipRequest("GET", "/ping", nil)
		if err != nil {
			return err
		}
		fmt.Println(string(data))
		return nil
	},
}

// --- notes ---

var clipNotesCmd = &cobra.Command{
	Use:   "notes",
	Short: "Manage notes",
}

var clipNotesListCmd = &cobra.Command{
	Use:   "list",
	Short: "List notes",
	RunE: func(cmd *cobra.Command, args []string) error {
		limit, _ := cmd.Flags().GetInt("limit")
		page, _ := cmd.Flags().GetInt("page")
		path := fmt.Sprintf("/notes?limit=%d&page=%d&fields=id,title,updated_time", limit, page)
		data, err := clipRequest("GET", path, nil)
		if err != nil {
			return err
		}
		var resp struct {
			Items []struct {
				ID          string `json:"id"`
				Title       string `json:"title"`
				UpdatedTime int64  `json:"updated_time"`
			} `json:"items"`
			HasMore bool `json:"has_more"`
		}
		if err := json.Unmarshal(data, &resp); err != nil {
			return err
		}
		tw := tabwriter.NewWriter(os.Stdout, 0, 0, 2, ' ', 0)
		fmt.Fprintln(tw, "ID\tTITLE\tUPDATED")
		for _, n := range resp.Items {
			t := time.UnixMilli(n.UpdatedTime).Format("2006-01-02 15:04")
			fmt.Fprintf(tw, "%s\t%s\t%s\n", n.ID, n.Title, t)
		}
		tw.Flush()
		if resp.HasMore {
			fmt.Printf("\n(more results available, use --page %d)\n", page+1)
		}
		return nil
	},
}

var clipNotesGetCmd = &cobra.Command{
	Use:   "get <id>",
	Short: "Get a note by ID",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		fields, _ := cmd.Flags().GetString("fields")
		path := "/notes/" + args[0]
		if fields != "" {
			path += "?fields=" + url.QueryEscape(fields)
		}
		data, err := clipRequest("GET", path, nil)
		if err != nil {
			return err
		}
		fmt.Println(prettyJSON(data))
		return nil
	},
}

var clipNotesCreateCmd = &cobra.Command{
	Use:   "create",
	Short: "Create a note",
	RunE: func(cmd *cobra.Command, args []string) error {
		title, _ := cmd.Flags().GetString("title")
		body, _ := cmd.Flags().GetString("body")
		folderID, _ := cmd.Flags().GetString("folder-id")
		tagIDs, _ := cmd.Flags().GetStringSlice("tag")
		if title == "" {
			return fmt.Errorf("--title is required")
		}
		payload := map[string]interface{}{"title": title}
		if body != "" {
			payload["body"] = body
		}
		if folderID != "" {
			payload["parent_id"] = folderID
		}
		if len(tagIDs) > 0 {
			payload["tag_ids"] = tagIDs
		}
		data, err := clipRequest("POST", "/notes", payload)
		if err != nil {
			return err
		}
		fmt.Println(prettyJSON(data))
		return nil
	},
}

var clipNotesDeleteCmd = &cobra.Command{
	Use:   "delete <id>",
	Short: "Delete a note by ID",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		_, err := clipRequest("DELETE", "/notes/"+args[0], nil)
		if err != nil {
			return err
		}
		fmt.Println("deleted")
		return nil
	},
}

// --- folders ---

var clipFoldersCmd = &cobra.Command{
	Use:   "folders",
	Short: "Manage folders",
}

var clipFoldersListCmd = &cobra.Command{
	Use:   "list",
	Short: "List folders",
	RunE: func(cmd *cobra.Command, args []string) error {
		data, err := clipRequest("GET", "/folders?fields=id,title,parent_id", nil)
		if err != nil {
			return err
		}
		var resp struct {
			Items []struct {
				ID       string `json:"id"`
				Title    string `json:"title"`
				ParentID string `json:"parent_id"`
			} `json:"items"`
		}
		if err := json.Unmarshal(data, &resp); err != nil {
			return err
		}
		tw := tabwriter.NewWriter(os.Stdout, 0, 0, 2, ' ', 0)
		fmt.Fprintln(tw, "ID\tTITLE\tPARENT")
		for _, f := range resp.Items {
			parent := f.ParentID
			if parent == "" {
				parent = "(root)"
			}
			fmt.Fprintf(tw, "%s\t%s\t%s\n", f.ID, f.Title, parent)
		}
		tw.Flush()
		return nil
	},
}

var clipFoldersCreateCmd = &cobra.Command{
	Use:   "create",
	Short: "Create a folder",
	RunE: func(cmd *cobra.Command, args []string) error {
		title, _ := cmd.Flags().GetString("title")
		parentID, _ := cmd.Flags().GetString("parent-id")
		if title == "" {
			return fmt.Errorf("--title is required")
		}
		payload := map[string]string{"title": title}
		if parentID != "" {
			payload["parent_id"] = parentID
		}
		data, err := clipRequest("POST", "/folders", payload)
		if err != nil {
			return err
		}
		fmt.Println(prettyJSON(data))
		return nil
	},
}

// --- tags ---

var clipTagsCmd = &cobra.Command{
	Use:   "tags",
	Short: "Manage tags",
}

var clipTagsListCmd = &cobra.Command{
	Use:   "list",
	Short: "List tags",
	RunE: func(cmd *cobra.Command, args []string) error {
		data, err := clipRequest("GET", "/tags?fields=id,title", nil)
		if err != nil {
			return err
		}
		var resp struct {
			Items []struct {
				ID    string `json:"id"`
				Title string `json:"title"`
			} `json:"items"`
		}
		if err := json.Unmarshal(data, &resp); err != nil {
			return err
		}
		tw := tabwriter.NewWriter(os.Stdout, 0, 0, 2, ' ', 0)
		fmt.Fprintln(tw, "ID\tTITLE")
		for _, t := range resp.Items {
			fmt.Fprintf(tw, "%s\t%s\n", t.ID, t.Title)
		}
		tw.Flush()
		return nil
	},
}

// --- search ---

var clipSearchCmd = &cobra.Command{
	Use:   "search <query>",
	Short: "Search notes, folders, or tags",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		searchType, _ := cmd.Flags().GetString("type")
		path := "/search?query=" + url.QueryEscape(args[0])
		if searchType != "" {
			path += "&type=" + url.QueryEscape(searchType)
		}
		data, err := clipRequest("GET", path, nil)
		if err != nil {
			return err
		}
		fmt.Println(prettyJSON(data))
		return nil
	},
}

func init() {
	clipCmd.PersistentFlags().StringVar(&clipServer, "server", "", "clipper server URL (default: http://localhost:41184)")

	// notes
	clipNotesListCmd.Flags().Int("limit", 20, "number of results per page")
	clipNotesListCmd.Flags().Int("page", 1, "page number")
	clipNotesGetCmd.Flags().String("fields", "", "comma-separated fields to return")
	clipNotesCreateCmd.Flags().String("title", "", "note title (required)")
	clipNotesCreateCmd.Flags().String("body", "", "note body in markdown")
	clipNotesCreateCmd.Flags().String("folder-id", "", "parent folder ID")
	clipNotesCreateCmd.Flags().StringSlice("tag", nil, "tag ID(s) to assign (can be repeated: --tag id1 --tag id2)")
	clipNotesCmd.AddCommand(clipNotesListCmd, clipNotesGetCmd, clipNotesCreateCmd, clipNotesDeleteCmd)

	// folders
	clipFoldersCreateCmd.Flags().String("title", "", "folder title (required)")
	clipFoldersCreateCmd.Flags().String("parent-id", "", "parent folder ID")
	clipFoldersCmd.AddCommand(clipFoldersListCmd, clipFoldersCreateCmd)

	// tags
	clipTagsCmd.AddCommand(clipTagsListCmd)

	// search
	clipSearchCmd.Flags().String("type", "", "search type: note, folder, tag (default: note)")

	clipCmd.AddCommand(clipNotesCmd, clipFoldersCmd, clipTagsCmd, clipSearchCmd, clipPingCmd)
	rootCmd.AddCommand(clipCmd)
}
