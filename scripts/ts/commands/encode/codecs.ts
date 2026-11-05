import bs58 from 'bs58'
import { bufferToNumber, numberToBuffer } from '../../utils/seeds.ts'

// --- Codec interface ---

interface Codec {
  name: string
  detect: (input: string) => boolean
  decode: (input: string) => Buffer
  encode: (bytes: Buffer) => string
  /** If defined, return false to skip this format in multi-format output */
  shouldDisplay?: (bytes: Buffer) => boolean
}

// --- Codecs ---

function trim0x(input: string): string {
  return input.startsWith('0x') ? input.slice(2) : input
}

const hexCodec: Codec = {
  name: 'hex',
  detect: (input) => /^[0-9a-fA-F]+$/.test(trim0x(input)) && input.length % 2 === 0,
  decode: (input) => Buffer.from(trim0x(input), 'hex'),
  encode: (bytes) => '0x' + Buffer.from(bytes).toString('hex'),
}

const base58Codec: Codec = {
  name: 'base58',
  detect: (input) => /^[1-9A-HJ-NP-Za-km-z]+$/.test(input),
  decode: (input) => Buffer.from(bs58.decode(input)),
  encode: (bytes) => bs58.encode(bytes),
}

const intArrayCodec: Codec = {
  name: 'intarray',
  detect: (input) => /^\[[\d\s,]+\]$/.test(input),
  decode: (input) => Buffer.from(JSON.parse(input) as number[]),
  encode: (bytes) => '[' + Array.from(bytes).join(', ') + ']',
}

const base64Codec: Codec = {
  name: 'base64',
  detect: (input) => /^[A-Za-z0-9+/]+=*$/.test(input) && /[+/=]/.test(input),
  decode: (input) => Buffer.from(input, 'base64'),
  encode: (bytes) => Buffer.from(bytes).toString('base64'),
}

const utf8Codec: Codec = {
  name: 'utf8',
  detect: () => true, // fallback — always matches last
  decode: (input) => Buffer.from(input, 'utf-8'),
  encode: (bytes) => Buffer.from(bytes).toString('utf-8'),
  shouldDisplay: (bytes) => {
    let printable = 0
    for (const byte of bytes) {
      if ((byte >= 0x20 && byte <= 0x7e) || byte === 0x0a || byte === 0x0d || byte === 0x09) {
        printable++
      }
    }
    return bytes.length > 0 && printable / bytes.length >= 0.9
  },
}

// --- Codec registry ---

const numberCodecs = [16, 32, 64].flatMap((bits) => [
  uintCodecFactory(bits, true),
  uintCodecFactory(bits, false),
])

// Registration order matters: first match wins for auto-detection.
// Put more specific detectors before broad ones (e.g. intarray before hex).
export const codecs: Codec[] = [
  intArrayCodec,
  hexCodec,
  base64Codec,
  base58Codec,
  ...numberCodecs,
  utf8Codec, // keep utf8 last as detection is just a fallback
]

export const codecNames = codecs.map((c) => c.name).join(', ')

export function findCodec(name: string): Codec {
  const codec = codecs.find((c) => c.name === name)
  if (!codec) {
    throw new Error(`Unknown format: ${name}. Valid: ${codecs.map((c) => c.name).join(', ')}`)
  }
  return codec
}

export function detectCodec(input: string): Codec {
  const trimmed = input.trim()
  return codecs.find((c) => c.detect(trimmed)) ?? utf8Codec
}

function uintCodecFactory(bitLength: number, littleEndian: boolean): Codec {
  return {
    name: `u${bitLength}${littleEndian ? 'le' : 'be'}`,
    detect: (input) =>
      /^-?\d+$/.test(input) && BigInt(input) >= 0n && BigInt(input) < 1n << BigInt(bitLength),
    decode: (input) => numberToBuffer(BigInt(input), bitLength, littleEndian, false),
    encode: (bytes) => bufferToNumber(bytes, bitLength, littleEndian, false).toString(),
    shouldDisplay: (bytes) => bytes.length * 8 === bitLength,
  }
}
