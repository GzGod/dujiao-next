package railwayapi

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"strings"
	"time"

	resellerdomain "github.com/dujiao-next/internal/modules/reseller/domain"
)

// Client is scoped to one project/environment/service. No user supplied endpoint is accepted.
type Client struct {
	Token, ProjectID, EnvironmentID, ServiceID string
	Port                                       int
	HTTP                                       *http.Client
}

type providerDomain struct {
	ID         string `json:"id"`
	Domain     string `json:"domain"`
	TargetPort int    `json:"targetPort"`
	Status     struct {
		Verified            bool   `json:"verified"`
		VerificationToken   string `json:"verificationToken"`
		VerificationDNSHost string `json:"verificationDnsHost"`
		CertificateStatus   string `json:"certificateStatus"`
		DNSRecords          []struct {
			Hostlabel, RequiredValue, Status, RecordType string
		} `json:"dnsRecords"`
	} `json:"status"`
}

const domainFields = `id domain targetPort status { verified verificationToken verificationDnsHost certificateStatus dnsRecords { hostlabel requiredValue status recordType } }`

func (c *Client) query(ctx context.Context, query string, variables interface{}, out interface{}) error {
	body, err := json.Marshal(map[string]interface{}{"query": query, "variables": variables})
	if err != nil {
		return errors.New("provider_unavailable")
	}
	request, err := http.NewRequestWithContext(ctx, http.MethodPost, "https://backboard.railway.com/graphql/v2", bytes.NewReader(body))
	if err != nil {
		return errors.New("provider_unavailable")
	}
	request.Header.Set("Project-Access-Token", c.Token)
	request.Header.Set("Content-Type", "application/json")
	request.Header.Set("User-Agent", "AIshop-domain-provisioner/1.0")
	client := c.HTTP
	if client == nil {
		client = &http.Client{Timeout: 25 * time.Second, CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }}
	}
	response, err := client.Do(request)
	if err != nil {
		return errors.New("provider_unavailable")
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusOK {
		return errors.New("provider_unavailable")
	}
	var envelope struct {
		Data   json.RawMessage   `json:"data"`
		Errors []json.RawMessage `json:"errors"`
	}
	if err := json.NewDecoder(io.LimitReader(response.Body, 1<<20)).Decode(&envelope); err != nil || len(envelope.Errors) > 0 || len(envelope.Data) == 0 {
		return errors.New("provider_unavailable")
	}
	if err := json.Unmarshal(envelope.Data, out); err != nil {
		return errors.New("provider_unavailable")
	}
	return nil
}

func (c *Client) EnsureDomain(ctx context.Context, host string) (string, *resellerdomain.DomainDNS, error) {
	// List before creating, including after an ambiguous previous create response.
	var result struct {
		Domains struct {
			CustomDomains []providerDomain `json:"customDomains"`
		} `json:"domains"`
	}
	vars := map[string]interface{}{"projectId": c.ProjectID, "environmentId": c.EnvironmentID, "serviceId": c.ServiceID}
	if err := c.query(ctx, `query($projectId:String!,$environmentId:String!,$serviceId:String!){domains(projectId:$projectId,environmentId:$environmentId,serviceId:$serviceId){customDomains{`+domainFields+`}}}`, vars, &result); err != nil {
		return "", nil, err
	}
	for _, domain := range result.Domains.CustomDomains {
		if strings.EqualFold(domain.Domain, host) {
			if domain.TargetPort != c.Port {
				return domain.ID, nil, errors.New("provider_unavailable")
			}
			return translate(domain)
		}
	}
	var created struct {
		Domain providerDomain `json:"customDomainCreate"`
	}
	input := map[string]interface{}{"projectId": c.ProjectID, "environmentId": c.EnvironmentID, "serviceId": c.ServiceID, "domain": host, "targetPort": c.Port}
	if err := c.query(ctx, `mutation($input:CustomDomainCreateInput!){customDomainCreate(input:$input){`+domainFields+`}}`, map[string]interface{}{"input": input}, &created); err != nil {
		return "", nil, err
	}
	if !strings.EqualFold(created.Domain.Domain, host) {
		return "", nil, errors.New("provider_unavailable")
	}
	return translate(created.Domain)
}

func translate(domain providerDomain) (string, *resellerdomain.DomainDNS, error) {
	dns := &resellerdomain.DomainDNS{TXTHost: domain.Status.VerificationDNSHost, TXTValue: domain.Status.VerificationToken, Status: "pending_dns"}
	valid := false
	for _, record := range domain.Status.DNSRecords {
		if record.RecordType != "DNS_RECORD_TYPE_CNAME" {
			continue
		}
		dns.CNAME = record.RequiredValue
		dns.RecordName = record.Hostlabel
		if dns.RecordName == "" {
			dns.RecordName = "@"
		}
		valid = record.Status == "DNS_RECORD_STATUS_PROPAGATED"
	}
	if domain.ID == "" || dns.CNAME == "" {
		return "", nil, errors.New("provider_unavailable")
	}
	if valid {
		dns.Status = "pending_tls"
	}
	if valid && domain.Status.Verified && domain.Status.CertificateStatus == "CERTIFICATE_STATUS_TYPE_VALID" {
		dns.Status = "ready"
	}
	if domain.Status.CertificateStatus == "CERTIFICATE_STATUS_TYPE_FAILED" {
		dns.Status = "error"
		dns.ErrorCode = "certificate_failed"
	}
	return domain.ID, dns, nil
}
