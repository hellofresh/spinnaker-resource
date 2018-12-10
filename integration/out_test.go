package integration_test

import (
	"bytes"
	"encoding/json"
	"net/http"
	"os/exec"
	"strconv"
	"time"

	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
	"github.com/onsi/gomega/gbytes"
	"github.com/onsi/gomega/gexec"
	"github.com/onsi/gomega/ghttp"

	"github.com/pivotal-cf/spinnaker-resource/concourse"
)

var _ = Describe("Out", func() {
	var (
		applicationName, pipelineName string
		pipelineExecutionID           string
		responseMap                   map[string]string
		input                         concourse.OutRequest
		marshalledInput               []byte
		err                           error
		outResponse                   concourse.OutResponse
		inputSource                   concourse.Source
		statusHandlers                []http.HandlerFunc
	)
	BeforeEach(func() {
		pipelineName = "foo"
		applicationName = "bar"
		inputSource = concourse.Source{
			SpinnakerAPI:         spinnakerServer.URL(),
			SpinnakerApplication: applicationName,
			SpinnakerPipeline:    pipelineName,
			X509Cert:             serverCert,
			X509Key:              serverKey,
		}
		pipelineExecutionID = "ABC123"

		spinnakerServer.AppendHandlers(
			ghttp.CombineHandlers(
				ghttp.VerifyRequest("GET", MatchRegexp(".*/applications/"+inputSource.SpinnakerApplication)),
				ghttp.RespondWithJSONEncoded(
					200,
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
				ghttp.VerifyRequest("GET", MatchRegexp(".*/applications/"+inputSource.SpinnakerApplication+"/pipelineConfigs")),
				ghttp.RespondWithJSONEncoded(
					200,
					[]map[string]string{
						{"name": pipelineName},
					},
				)),
		)

	})
	JustBeforeEach(func() {
		input = concourse.OutRequest{
			Source: inputSource,
		}
		marshalledInput, err = json.Marshal(input)
		Expect(err).ToNot(HaveOccurred())
		spinnakerServer.AppendHandlers(statusHandlers...)
	})

	Context("when Spinnaker responds with an accepted pipeline execution", func() {
		BeforeEach(func() {
			httpPOSTSuccessHandler := ghttp.CombineHandlers(
				ghttp.VerifyRequest("POST", MatchRegexp(".*/pipelines/"+inputSource.SpinnakerApplication+"/"+pipelineName+".*")),
				ghttp.RespondWithJSONEncoded(
					202,
					map[string]string{
						"ref": "/pipelines/" + pipelineExecutionID,
					},
				),
			)
			spinnakerServer.AppendHandlers(httpPOSTSuccessHandler)
		})

		It("returns the pipeline execution id", func() {
			cmd := exec.Command(outPath)
			cmd.Stdin = bytes.NewBuffer(marshalledInput)
			outSess, err := gexec.Start(cmd, GinkgoWriter, GinkgoWriter)
			Expect(err).ToNot(HaveOccurred())
			<-outSess.Exited
			Expect(outSess.ExitCode()).To(Equal(0))

			err = json.Unmarshal(outSess.Out.Contents(), &outResponse)
			Expect(err).ToNot(HaveOccurred())
			Expect(outResponse.Version.Ref).To(Equal(pipelineExecutionID))
		})
		Context("when status is configured", func() {
			BeforeEach(func() {
				inputSource.Statuses = []string{"SUCCEEDED"}
				inputSource.StatusCheckInterval = 200 * time.Millisecond
			})

			Context("when a status and timeout are specified and Spinnaker pipeline doesn't reach the desired state within the timeout duration", func() {
				BeforeEach(func() {
					inputSource.StatusCheckTimeout = 500 * time.Millisecond

					runningHandler := ghttp.CombineHandlers(
						ghttp.VerifyRequest("GET", MatchRegexp(".*/pipelines/"+pipelineExecutionID+".*")),
						ghttp.RespondWithJSONEncoded(
							200,
							map[string]string{
								"id":     pipelineExecutionID,
								"status": "RUNNING",
							},
						),
					)
					statusHandlers = []http.HandlerFunc{
						runningHandler,
						runningHandler,
						runningHandler,
					}
				})

				It("times out and exits with a non zero status and prints an error message", func() {
					cmd := exec.Command(outPath)
					cmd.Stdin = bytes.NewBuffer(marshalledInput)
					outSess, err := gexec.Start(cmd, GinkgoWriter, GinkgoWriter)
					Expect(err).ToNot(HaveOccurred())
					Eventually(outSess.Exited).Should(BeClosed())
					Expect(outSess.ExitCode()).To(Equal(1))

					Expect(outSess.Err).To(gbytes.Say("error put step failed: "))
					Expect(outSess.Err).To(gbytes.Say("timed out waiting for configured status\\(es\\)"))
				})
			})
			Context("when a status is specified, and an unexpected final status reached", func() {
				BeforeEach(func() {
					statusHandlers := []http.HandlerFunc{
						ghttp.CombineHandlers(
							ghttp.VerifyRequest("GET", MatchRegexp(".*/pipelines/"+pipelineExecutionID+".*")),
							ghttp.RespondWithJSONEncoded(
								200,
								map[string]string{
									"id":     pipelineExecutionID,
									"status": "RUNNING",
								},
							),
						),
						ghttp.CombineHandlers(
							ghttp.VerifyRequest("GET", MatchRegexp(".*/pipelines/"+pipelineExecutionID+".*")),
							ghttp.RespondWithJSONEncoded(
								200,
								map[string]string{
									"id":     pipelineExecutionID,
									"status": "TERMINAL",
								},
							),
						),
					}
					spinnakerServer.AppendHandlers(statusHandlers...)
				})

				It("exits with non zero code and prints an error message", func() {
					cmd := exec.Command(outPath)
					cmd.Stdin = bytes.NewBuffer(marshalledInput)
					outSess, err := gexec.Start(cmd, GinkgoWriter, GinkgoWriter)
					Expect(err).ToNot(HaveOccurred())
					<-outSess.Exited
					Expect(spinnakerServer.ReceivedRequests()).Should(HaveLen(5))
					Expect(outSess.ExitCode()).To(Equal(1))

					Expect(outSess.Err).To(gbytes.Say("error put step failed:"))
					Expect(outSess.Err).To(gbytes.Say("Pipeline execution reached a final state: TERMINAL"))
				})
			})

			//TODO needs drying up
			Context("when a status is specified, and reached", func() {
				BeforeEach(func() {
					statusHandlers = []http.HandlerFunc{
						ghttp.CombineHandlers(
							ghttp.VerifyRequest("GET", MatchRegexp(".*/pipelines/"+pipelineExecutionID+".*")),
							ghttp.RespondWithJSONEncoded(
								200,
								map[string]string{
									"id":     pipelineExecutionID,
									"status": "RUNNING",
								},
							),
						),
						ghttp.CombineHandlers(
							ghttp.VerifyRequest("GET", MatchRegexp(".*/pipelines/"+pipelineExecutionID+".*")),
							ghttp.RespondWithJSONEncoded(
								200,
								map[string]string{
									"id":     pipelineExecutionID,
									"status": "SUCCEEDED",
								},
							),
						),
					}
				})
				It("waits till the pipeline execution status is satisfied and returns the pipeline execution id", func() {
					cmd := exec.Command(outPath)
					cmd.Stdin = bytes.NewBuffer(marshalledInput)
					outSess, err := gexec.Start(cmd, GinkgoWriter, GinkgoWriter)
					Expect(err).ToNot(HaveOccurred())
					<-outSess.Exited
					Expect(spinnakerServer.ReceivedRequests()).Should(HaveLen(5))
					Expect(outSess.ExitCode()).To(Equal(0))

					err = json.Unmarshal(outSess.Out.Contents(), &outResponse)
					Expect(err).ToNot(HaveOccurred())
					Expect(outResponse.Version.Ref).To(Equal(pipelineExecutionID))
				})
			})
		})
	})

	Context("when Spinnaker responds with 4xx on a POST for a pipeline execution", func() {
		var statusCode int
		BeforeEach(func() {

			statusCode = 422
			responseMap = map[string]string{
				"message": "500 ",
			}
			httpPOSTFailureHandler := ghttp.CombineHandlers(
				ghttp.VerifyRequest("POST", MatchRegexp(".*/pipelines/"+inputSource.SpinnakerApplication+"/"+pipelineName+".*")),
				ghttp.RespondWithJSONEncoded(
					statusCode,
					responseMap,
				),
			)
			spinnakerServer.AppendHandlers(httpPOSTFailureHandler)

		})

		It("prints the status code, response body and exits with exit code 1", func() {
			cmd := exec.Command(outPath)
			cmd.Stdin = bytes.NewBuffer(marshalledInput)
			outSess, err := gexec.Start(cmd, GinkgoWriter, GinkgoWriter)
			Expect(err).ToNot(HaveOccurred())
			<-outSess.Exited
			Expect(outSess.ExitCode()).To(Equal(1))

			Expect(outSess.Err).To(gbytes.Say("error put step failed:"))
			Expect(outSess.Err).To(gbytes.Say("spinnaker api responded with status code: " + strconv.Itoa(statusCode)))
			responseString, err := json.Marshal(responseMap)
			Expect(err).ToNot(HaveOccurred())
			Expect(outSess.Err).To(gbytes.Say("body: " + string(responseString)))
		})
	})
})
