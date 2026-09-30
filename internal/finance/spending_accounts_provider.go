package finance

import (
	"context"
	"time"

	"github.com/plaid/plaid-go/v43/plaid"
)

type bankAccountRecord struct{ ID, Name, Mask, Type, Subtype string }
type bankAccountInventory struct {
	Institution string
	Accounts    []bankAccountRecord
}
type hostedAccountLink struct {
	Token, URL string
	ExpiresAt  time.Time
}
type linkedAccountItem struct{ ID, AccessToken string }

type spendingAccountProvider interface {
	Accounts(context.Context, string) (bankAccountInventory, error)
	StartLink(context.Context) (hostedAccountLink, error)
	PublicToken(context.Context, string) (string, error)
	Exchange(context.Context, string) (linkedAccountItem, error)
}

func (s *SpendingService) newAccountProvider() (spendingAccountProvider, error) {
	if s.accountProviderFactory != nil {
		return s.accountProviderFactory()
	}
	client, err := plaidClient()
	if err != nil {
		return nil, err
	}
	return plaidSpendingAccountProvider{client: client}, nil
}

type plaidSpendingAccountProvider struct{ client *plaid.APIClient }

func (p plaidSpendingAccountProvider) Accounts(ctx context.Context, token string) (bankAccountInventory, error) {
	request := plaid.NewAccountsGetRequest(token)
	response, _, err := p.client.PlaidApi.AccountsGet(ctx).AccountsGetRequest(*request).Execute()
	if err != nil {
		return bankAccountInventory{}, err
	}
	result := bankAccountInventory{Accounts: []bankAccountRecord{}}
	for _, account := range response.GetAccounts() {
		result.Accounts = append(result.Accounts, bankAccountRecord{ID: account.GetAccountId(), Name: account.GetName(), Mask: account.GetMask(), Type: string(account.GetType()), Subtype: string(account.GetSubtype())})
	}
	item := response.GetItem()
	institutionID := item.GetInstitutionId()
	if institutionID != "" {
		request := plaid.NewInstitutionsGetByIdRequest(institutionID, []plaid.CountryCode{plaid.COUNTRYCODE_US})
		response, _, err := p.client.PlaidApi.InstitutionsGetById(ctx).InstitutionsGetByIdRequest(*request).Execute()
		if err == nil {
			institution := response.GetInstitution()
			result.Institution = institution.GetName()
		}
	}
	return result, ctx.Err()
}

func (p plaidSpendingAccountProvider) StartLink(ctx context.Context) (hostedAccountLink, error) {
	request := plaid.NewLinkTokenCreateRequest("finance", "en", []plaid.CountryCode{plaid.COUNTRYCODE_US})
	request.SetUser(*plaid.NewLinkTokenCreateRequestUser("david"))
	request.SetProducts([]plaid.Products{plaid.PRODUCTS_TRANSACTIONS})
	request.SetEnableMultiItemLink(false)
	hosted := plaid.NewLinkTokenCreateHostedLink()
	hosted.SetUrlLifetimeSeconds(1800)
	request.SetHostedLink(*hosted)
	response, _, err := p.client.PlaidApi.LinkTokenCreate(ctx).LinkTokenCreateRequest(*request).Execute()
	if err != nil {
		return hostedAccountLink{}, err
	}
	return hostedAccountLink{Token: response.GetLinkToken(), URL: response.GetHostedLinkUrl(), ExpiresAt: response.GetExpiration()}, nil
}

func (p plaidSpendingAccountProvider) PublicToken(ctx context.Context, token string) (string, error) {
	request := plaid.NewLinkTokenGetRequest(token)
	response, _, err := p.client.PlaidApi.LinkTokenGet(ctx).LinkTokenGetRequest(*request).Execute()
	if err != nil {
		return "", err
	}
	for _, session := range response.GetLinkSessions() {
		if results, ok := session.GetResultsOk(); ok {
			for _, item := range results.GetItemAddResults() {
				if token := item.GetPublicToken(); token != "" {
					return token, nil
				}
			}
		}
	}
	return "", nil
}

func (p plaidSpendingAccountProvider) Exchange(ctx context.Context, token string) (linkedAccountItem, error) {
	request := plaid.NewItemPublicTokenExchangeRequest(token)
	response, _, err := p.client.PlaidApi.ItemPublicTokenExchange(ctx).ItemPublicTokenExchangeRequest(*request).Execute()
	if err != nil {
		return linkedAccountItem{}, err
	}
	return linkedAccountItem{ID: response.GetItemId(), AccessToken: response.GetAccessToken()}, nil
}
