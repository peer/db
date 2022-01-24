package main

import (
	"context"
	"fmt"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/hashicorp/go-retryablehttp"
	"github.com/olivere/elastic/v7"
	"gitlab.com/tozd/go/errors"
	"gitlab.com/tozd/go/mediawiki"
	"gitlab.com/tozd/go/x"
)

var (
	// TODO: Configure logger.
	client   = retryablehttp.NewClient()
	esClient *elastic.Client
)

// TODO: Generate automatically.
const indexConfiguration = `{
	"settings":{
		"number_of_shards": 1,
		"number_of_replicas": 0
	},
	"mappings": {
		"dynamic": false,
		"dynamic_templates": [
			{
				"scores_float": {
					"match_mapping_type": "float",
					"mapping": {
						"type": "double"
					}
				},
				"scores_integer": {
					"match_mapping_type": "integer",
					"mapping": {
						"type": "double"
					}
				},
				"scores_long": {
					"match_mapping_type": "long",
					"mapping": {
						"type": "double"
					}
				}
			}
		],
		"properties": {
			"name": {
				"properties": {
						"en": {
							"type": "text"
							"analyzer: "english"
						}
					}
				}
			},
			"otherNames": {
				"properties": {
					"en": {
						"type": "text"
						"analyzer: "english"
					}
				}
			},
			"score": {
				"type": "double"
			},
			"scores": {
				"dynamic": true,
				"properties": {}
			}
			"mnemonic": {
				"type": "keyword",
				"doc_values": false
			},
			"active": {
				"properties": {
					"id": {
						"type": "nested",
						"properties": {
							"_id": {
								"type": "keyword",
								"doc_values": false
							},
							"confidence": {
								"type": "double"
							},
							"prop": {
								"properties": {

								}
							},
							"id": {
								"type": "keyword",
							}
						}
					},
					"ref": {},
					"text": {},
					"string": {},
					"label": {},
					"amount": {},
					"amountRange": {},
					"enum": {},
					"rel": {},
					"none": {},
					"unknown": {},
					"time": {},
					"timeRange": {},
					"duration": {},
					"durationRange": {},
					"file": {},
					"list": {}
				}
			}
		}
	}
}`

func init() {
	var err error
	esClient, err = elastic.NewClient()
	if err != nil {
		panic(err)
	}
}

func convert(config *Config) errors.E {
	ctx := context.Background()

	// We call cancel on SIGINT or SIGTERM signal.
	ctx, cancel := context.WithCancel(ctx)
	defer cancel()

	// Call cancel on SIGINT or SIGTERM signal.
	go func() {
		c := make(chan os.Signal, 1)
		defer close(c)

		signal.Notify(c, os.Interrupt, syscall.SIGTERM)
		defer signal.Stop(c)

		// We wait for a signal or that the context is canceled
		// or that all goroutines are done.
		select {
		case <-c:
			cancel()
		case <-ctx.Done():
		}
	}()

	_, _, err := esClient.Ping(elastic.DefaultURL).Do(ctx)
	if err != nil {
		return errors.WithStack(err)
	}

	exists, err := esClient.IndexExists("docs").Do(ctx)
	if err != nil {
		return errors.WithStack(err)
	}

	if !exists {
		createIndex, err := esClient.CreateIndex("docs").BodyString(mapping).Do(ctx)
		if err != nil {
			return errors.WithStack(err)
		}
		if !createIndex.Acknowledged {
			return errors.New("create index not acknowledged")
		}
	}

	// TODO: Make number of workers configurable.
	processor, err := esClient.BulkProcessor().Workers(2).Stats(true).After(
		func(executionId int64, requests []elastic.BulkableRequest, response *elastic.BulkResponse, err error) {
			if err != nil {
				fmt.Fprintf(os.Stderr, "Indexing error: %s\n", err.Error())
			}
		},
	).Do(ctx)
	if err != nil {
		return errors.WithStack(err)
	}
	defer processor.Close()

	return mediawiki.ProcessWikidataDump(ctx, &mediawiki.ProcessDumpConfig{
		URL:                    "",
		CacheDir:               config.CacheDir,
		Client:                 client,
		DecompressionThreads:   0,
		JSONDecodeThreads:      0,
		ItemsProcessingThreads: 0,
		// TODO: Make contact e-mail into a CLI argument.
		UserAgent: fmt.Sprintf("PeerBot/%s (build on %s, git revision %s) (mailto:mitar.peerbot@tnode.com)", version, buildTimestamp, revision),
		Progress: func(ctx context.Context, p x.Progress) {
			stats := processor.Stats()
			fmt.Fprintf(os.Stderr, "Progress: %0.2f%%, ETA: %s, processed: %d, indexed: %d, failed: %d\n", p.Percent(), p.Remaining().Truncate(time.Second), p.Count, stats.Indexed, stats.Failed)
		},
	}, func(ctx context.Context, entity mediawiki.Entity) errors.E {
		return processEntity(ctx, config, processor, entity)
	})
}
