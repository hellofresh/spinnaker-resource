package integration_test

import (
	"bytes"
	"encoding/json"
	"net/http"
	"os/exec"

	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
	"github.com/onsi/gomega/gexec"
	"github.com/onsi/gomega/ghttp"

	"github.com/pivotal-cf/spinnaker-resource/concourse"
)

var _ = Describe("Check", func() {
	var (
		applicationName, pipelineName string
		responseMap                   []map[string]interface{}
		input                         concourse.CheckRequest
		marshalledInput               []byte
		err                           error
		statusCode                    int
		pipelineExecutions            []map[string]interface{}
		checkResponse                 []concourse.Version
		allHandler                    http.HandlerFunc
		inputRef                      string
		checkSess                     *gexec.Session
	)
	pipelineName = "foo"
	applicationName = "bar"
	pipelineExecutions = []map[string]interface{}{
		map[string]interface{}{
			"id":        "EX1",
			"name":      pipelineName,
			"buildTime": 1543244670,
		},
		map[string]interface{}{
			"id":        "EX2",
			"name":      pipelineName,
			"buildTime": 1543244680,
		},
		map[string]interface{}{
			"id":        "EX3",
			"name":      pipelineName,
			"buildTime": 1543244690,
		},
		map[string]interface{}{
			"id":        "EX4",
			"name":      "other-pipeline",
			"buildTime": 1543244690,
		},
		map[string]interface{}{
			"id":        "EX5",
			"name":      "other-pipeline",
			"buildTime": 1543244690,
		},
	}
	JustBeforeEach(func() {
		spinnakerServer.AppendHandlers(
			ghttp.CombineHandlers(
				ghttp.VerifyRequest("GET", MatchRegexp(".*/applications/"+applicationName)),
				ghttp.RespondWithJSONEncoded(
					statusCode,
					map[string]interface{}{
						"attributes": map[string]interface{}{
							"accounts": nil,
							"name":     applicationName,
						},
						"clusters": nil,
						"name":     applicationName,
					},
				)),
			ghttp.CombineHandlers(
				ghttp.VerifyRequest("GET", MatchRegexp(".*/applications/"+applicationName+"/pipelineConfigs")),
				ghttp.RespondWithJSONEncoded(
					statusCode,
					[]map[string]string{
						{"name": pipelineName},
					},
				)),
			allHandler,
		)
		input = concourse.CheckRequest{
			Source: concourse.Source{
				SpinnakerAPI:         spinnakerServer.URL(),
				SpinnakerApplication: applicationName,
				SpinnakerPipeline:    pipelineName,
				X509Cert:             serverCert,
				X509Key:              serverKey,
			},
			Version: concourse.Version{
				Ref: inputRef,
			},
		}
		marshalledInput, err = json.Marshal(input)
		Expect(err).ToNot(HaveOccurred())
		cmd := exec.Command(checkPath)
		cmd.Stdin = bytes.NewBuffer(marshalledInput)
		checkSess, err = gexec.Start(cmd, GinkgoWriter, GinkgoWriter)
		Expect(err).ToNot(HaveOccurred())
		<-checkSess.Exited
	})
	Context("when input version is not empty", func() {
		BeforeEach(func() {
			statusCode = 200
			allHandler = ghttp.CombineHandlers(
				ghttp.VerifyRequest("GET", MatchRegexp(".*/applications/"+applicationName+"/pipelines"), "limit=25"),
				ghttp.RespondWithJSONEncoded(
					statusCode,
					pipelineExecutions,
				),
			)
		})
		Context("when input version exists but not the latest version", func() {
			BeforeEach(func() {
				inputRef = pipelineExecutions[1]["id"].(string)
			})
			It("returns the input version and every version that follows", func() {
				Expect(checkSess.ExitCode()).To(Equal(0))

				err = json.Unmarshal(checkSess.Out.Contents(), &checkResponse)
				Expect(err).ToNot(HaveOccurred())
				Expect(len(checkResponse)).To(Equal(2))
				Expect(checkResponse[0].Ref).To(Equal(pipelineExecutions[1]["id"].(string)))
				Expect(checkResponse[1].Ref).To(Equal(pipelineExecutions[2]["id"].(string)))
			})
		})

		Context("when input version is the latest version", func() {
			BeforeEach(func() {
				inputRef = pipelineExecutions[2]["id"].(string)
			})
			It("returns the only the input version", func() {
				Expect(checkSess.ExitCode()).To(Equal(0))

				err = json.Unmarshal(checkSess.Out.Contents(), &checkResponse)
				Expect(err).ToNot(HaveOccurred())
				Expect(len(checkResponse)).To(Equal(1))
				Expect(checkResponse[0].Ref).To(Equal(pipelineExecutions[2]["id"].(string)))
			})
		})
		Context("when input version doesn't exist anymore", func() {
			BeforeEach(func() {
				inputRef = pipelineExecutions[0]["id"].(string)
			})
			It("returns the only the input version", func() {
				Expect(checkSess.ExitCode()).To(Equal(0))

				err = json.Unmarshal(checkSess.Out.Contents(), &checkResponse)
				Expect(err).ToNot(HaveOccurred())
				Expect(len(checkResponse)).To(Equal(1))
				Expect(checkResponse[0].Ref).To(Equal(pipelineExecutions[2]["id"].(string)))
			})
		})
	})
	Context("when input version is empty", func() {
		BeforeEach(func() {
			inputRef = ""
			pipelineName = "foo"
			applicationName = "bar"
			responseMap = []map[string]interface{}{
				pipelineExecutions[0],
				pipelineExecutions[1],
				pipelineExecutions[3],
				pipelineExecutions[4],
			}
			statusCode = 200
			allHandler = ghttp.CombineHandlers(
				ghttp.VerifyRequest("GET", MatchRegexp(".*/applications/"+applicationName+"/pipelines"), "limit=25"),
				ghttp.RespondWithJSONEncoded(
					statusCode,
					responseMap,
				),
			)
		})
		It("returns the only the latest version to stdout", func() {
			Expect(checkSess.ExitCode()).To(Equal(0))

			err = json.Unmarshal(checkSess.Out.Contents(), &checkResponse)
			Expect(err).ToNot(HaveOccurred())
			Expect(len(checkResponse)).To(Equal(1))
			Expect(checkResponse[0].Ref).To(Equal(pipelineExecutions[1]["id"].(string)))
		})
	})
})
