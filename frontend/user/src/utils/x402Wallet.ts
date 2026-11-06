// x402 钱包签名：通过浏览器注入钱包（window.ethereum，EIP-1193）对 EIP-3009
// TransferWithAuthorization 做 EIP-712 签名，生成 x402 v2 PaymentPayload。
// 只请求签名，不发交易、不要求付款人持有 gas；链上结算由服务端交给 facilitator 完成。

export interface X402Requirements {
  scheme: string
  network: string
  amount: string
  asset: string
  payTo: string
  maxTimeoutSeconds: number
  extra?: { name?: string; version?: string }
}

interface Eip1193Provider {
  request(args: { method: string; params?: unknown[] }): Promise<any>
}

export type X402WalletErrorCode = 'no_wallet' | 'rejected' | 'wrong_network' | 'failed'

export class X402WalletError extends Error {
  code: X402WalletErrorCode

  constructor(code: X402WalletErrorCode, message?: string) {
    super(message || code)
    this.code = code
  }
}

const NETWORK_PARAMS: Record<string, { chainName: string; rpcUrls: string[]; blockExplorerUrls: string[] }> = {
  'eip155:84532': {
    chainName: 'Base Sepolia',
    rpcUrls: ['https://sepolia.base.org'],
    blockExplorerUrls: ['https://sepolia.basescan.org'],
  },
  'eip155:8453': {
    chainName: 'Base',
    rpcUrls: ['https://mainnet.base.org'],
    blockExplorerUrls: ['https://basescan.org'],
  },
}

export const explorerTxUrl = (network: string, tx: string) => {
  const base = NETWORK_PARAMS[network]?.blockExplorerUrls[0]
  return base && tx ? `${base}/tx/${tx}` : ''
}

export const networkLabel = (network: string) => NETWORK_PARAMS[network]?.chainName || network

const getProvider = (): Eip1193Provider => {
  const provider = (window as any).ethereum as Eip1193Provider | undefined
  if (!provider || typeof provider.request !== 'function') {
    throw new X402WalletError('no_wallet')
  }
  return provider
}

const isUserRejection = (err: any) => err?.code === 4001 || err?.code === 'ACTION_REJECTED'

const chainIdOf = (network: string) => {
  const [namespace, reference] = network.split(':')
  const id = Number(reference)
  if (namespace !== 'eip155' || !Number.isInteger(id)) {
    throw new X402WalletError('wrong_network', `unsupported network ${network}`)
  }
  return id
}

const ensureChain = async (provider: Eip1193Provider, network: string) => {
  const chainId = chainIdOf(network)
  const hexId = `0x${chainId.toString(16)}`
  const current = await provider.request({ method: 'eth_chainId' })
  if (String(current).toLowerCase() === hexId) return
  try {
    await provider.request({ method: 'wallet_switchEthereumChain', params: [{ chainId: hexId }] })
  } catch (err: any) {
    if (isUserRejection(err)) throw new X402WalletError('rejected')
    const params = NETWORK_PARAMS[network]
    if (err?.code !== 4902 || !params) throw new X402WalletError('wrong_network', err?.message)
    await provider.request({
      method: 'wallet_addEthereumChain',
      params: [{ chainId: hexId, nativeCurrency: { name: 'Ether', symbol: 'ETH', decimals: 18 }, ...params }],
    })
  }
}

const randomNonce = () => {
  const bytes = new Uint8Array(32)
  crypto.getRandomValues(bytes)
  return `0x${Array.from(bytes, (b) => b.toString(16).padStart(2, '0')).join('')}`
}

/** 连接钱包并对收款要求签名，返回可直接提交给后端的 x402 PaymentPayload。 */
export async function signX402Payment(requirements: X402Requirements) {
  const provider = getProvider()
  let accounts: string[]
  try {
    accounts = await provider.request({ method: 'eth_requestAccounts' })
  } catch (err: any) {
    throw new X402WalletError(isUserRejection(err) ? 'rejected' : 'failed', err?.message)
  }
  const from = accounts?.[0]
  if (!from) throw new X402WalletError('no_wallet')

  await ensureChain(provider, requirements.network)

  const now = Math.floor(Date.now() / 1000)
  const authorization = {
    from,
    to: requirements.payTo,
    value: requirements.amount,
    validAfter: String(now - 600),
    validBefore: String(now + Math.max(60, Number(requirements.maxTimeoutSeconds) || 600)),
    nonce: randomNonce(),
  }
  const typedData = {
    types: {
      EIP712Domain: [
        { name: 'name', type: 'string' },
        { name: 'version', type: 'string' },
        { name: 'chainId', type: 'uint256' },
        { name: 'verifyingContract', type: 'address' },
      ],
      TransferWithAuthorization: [
        { name: 'from', type: 'address' },
        { name: 'to', type: 'address' },
        { name: 'value', type: 'uint256' },
        { name: 'validAfter', type: 'uint256' },
        { name: 'validBefore', type: 'uint256' },
        { name: 'nonce', type: 'bytes32' },
      ],
    },
    primaryType: 'TransferWithAuthorization',
    domain: {
      name: requirements.extra?.name || 'USDC',
      version: requirements.extra?.version || '2',
      chainId: chainIdOf(requirements.network),
      verifyingContract: requirements.asset,
    },
    message: authorization,
  }

  let signature: string
  try {
    signature = await provider.request({
      method: 'eth_signTypedData_v4',
      params: [from, JSON.stringify(typedData)],
    })
  } catch (err: any) {
    throw new X402WalletError(isUserRejection(err) ? 'rejected' : 'failed', err?.message)
  }

  return {
    x402Version: 2,
    accepted: requirements,
    payload: { signature, authorization },
  }
}
