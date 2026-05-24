package cmd

import (
	"fmt"
	"net/http"

	"github.com/spf13/cobra"
	"github.com/venkatvghub/argus/pkg/server"
)

var serveAddr string

var serveCmd = &cobra.Command{
	Use:   "serve",
	Short: "Server commands",
}

var serveRESTCmd = &cobra.Command{
	Use:   "rest",
	Short: "Start the REST HTTP server",
	RunE: func(cmd *cobra.Command, args []string) error {
		srv := server.NewRESTServer(instance)
		fmt.Printf("starting REST server on %s\n", serveAddr)
		return http.ListenAndServe(serveAddr, srv.Routes())
	},
}

var serveMCPCmd = &cobra.Command{
	Use:   "mcp",
	Short: "Start the MCP stdio server",
	RunE: func(cmd *cobra.Command, args []string) error {
		srv := server.NewMCPServer(instance)
		return srv.Run()
	},
}

func init() {
	serveRESTCmd.Flags().StringVar(&serveAddr, "addr", ":8080", "address to listen on")

	serveCmd.AddCommand(serveRESTCmd)
	serveCmd.AddCommand(serveMCPCmd)
}
