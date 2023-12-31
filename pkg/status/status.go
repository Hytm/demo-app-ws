package status

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"sort"
	"time"

	"github.com/jackc/pgx/v4/pgxpool"
)

type node struct {
	Nodes []struct {
		Name       string `json:"name"`
		RegionName string `json:"region_name"`
		Status     string `json:"status"`
	} `json:"nodes"`
	Pagination interface{} `json:"pagination"`
}

type Status struct {
	nodesPerState map[string]int
}

var isLocal bool
var dbpool *pgxpool.Pool

func NewStatus(local bool, cnx string) *Status {
	isLocal = local

	if isLocal {
		log.Println("Running in local mode")
		config, err := pgxpool.ParseConfig(cnx)
		if err != nil {
			log.Fatal("error configuring the database: ", err)
		}

		dbpool, err = pgxpool.ConnectConfig(context.Background(), config)
		if err != nil {
			log.Fatal("error connecting to the database: ", err)
		}
	} else {
		log.Println("Running in dedicated mode")
	}

	return &Status{
		nodesPerState: make(map[string]int),
	}
}

func (s *Status) RunHealthCheck() {
	for {
		node := getNodesInformation()
		if node == nil {
			return
		}
		//Clean the map
		for k := range s.nodesPerState {
			delete(s.nodesPerState, k)
		}

		for _, n := range node.Nodes {
			s.nodesPerState[n.Status]++
		}
		time.Sleep(250 * time.Millisecond)
	}
}

func (s *Status) GetStatus() string {
	message := ""
	notAlive := 0
	for k := range s.nodesPerState {
		if k != "LIVE" {
			notAlive++
		}
	}
	message = fmt.Sprintf("%d node(s) LIVE", s.nodesPerState["LIVE"])
	if notAlive != 0 {
		message = ""
		keys := []string{}
		for k, _ := range s.nodesPerState {
			keys = append(keys, k)
		}
		sort.Strings(keys)

		for _, k := range keys {
			message += fmt.Sprintf("| %d node(s) %s |", s.nodesPerState[k], k)
		}
	}

	return message
}

func getNodesInformation() *node {
	if isLocal {
		return getLocalStatus()
	}
	return getDedicatedStatus()
}

func getLocalStatus() *node {
	node := &node{}

	//select node_id, is_live from crdb_internal.gossip_nodes
	rows, err := dbpool.Query(context.Background(), "SELECT node_id, is_live FROM crdb_internal.gossip_nodes WHERE ranges > 0")
	if err != nil {
		log.Printf("client: could not query database: %s\n", err)
		return nil
	}
	defer rows.Close()

	for rows.Next() {
		var nodeID int
		var isLive bool
		err := rows.Scan(&nodeID, &isLive)
		if err != nil {
			log.Printf("client: could not scan row: %s\n", err)
			return nil
		}
		if isLive {
			node.Nodes = append(node.Nodes, struct {
				Name       string `json:"name"`
				RegionName string `json:"region_name"`
				Status     string `json:"status"`
			}{
				Name:       fmt.Sprintf("node%d", nodeID),
				RegionName: "local",
				Status:     "LIVE",
			})
		} else {
			node.Nodes = append(node.Nodes, struct {
				Name       string `json:"name"`
				RegionName string `json:"region_name"`
				Status     string `json:"status"`
			}{
				Name:       fmt.Sprintf("node%d", nodeID),
				RegionName: "local",
				Status:     "DEAD",
			})
		}
	}

	return node
}

func getDedicatedStatus() *node {
	url := os.Getenv("NODES_INFO_URL")
	req, err := http.NewRequest(http.MethodGet, url, nil)
	if err != nil {
		log.Printf("client: could not create request: %s\n", err)
		return nil
	}

	req.Header.Set("Authorization", os.Getenv("BEARER"))

	res, err := http.DefaultClient.Do(req)
	if err != nil {
		log.Printf("client: error making http request: %s\n", err)
		return nil
	}

	if res.StatusCode != http.StatusOK {
		log.Printf("client: unexpected status code: %d\n", res.StatusCode)
		return nil
	}

	resBody, err := io.ReadAll(res.Body)
	if err != nil {
		log.Printf("client: could not read response body: %s\n", err)
		return nil
	}

	node := &node{}
	err = json.Unmarshal(resBody, node)
	if err != nil {
		log.Printf("client: could not unmarshal json: %s\n", err)
		return nil
	}

	return node
}
