import { PublicKey } from '@solana/web3.js'
import type { Config } from '../index.ts'

const config: Config = {
  rpcUrl: 'https://api.mainnet-beta.solana.com',
  explorerUrl: (sig) => `https://explorer.solana.com/tx/${sig}`,
  ccipVersion: '1.6',
  programs: {
    router: new PublicKey('Ccip842gzYHhvdDkSyi2YVCoAWPbYJoApMFzSxQroE9C'),
    feeQuoter: new PublicKey('FeeQPGkKDeRV1MgoYfMH6L8o3KeuYjwUZrgn4LRKfjHi'),
    burnmintTokenPool: new PublicKey('41FGToCmdaWa1dgZLKFAjvmx6e6AjVTX7SVRibvsMGVB'),
    lockReleaseTokenPool: new PublicKey('8eqh8wppT9c5rw4ERqNCffvU6cNFJWff9WmkcYtmGiqC'),
    rmn: new PublicKey('RmnXLft1mSEwDgMKu2okYuHkiazxntFFcZFrrcXxYg7'),
  },
}

export default config
