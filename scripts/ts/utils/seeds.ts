import { PublicKey } from '@solana/web3.js'

export function parseSeed(seed: string): Buffer<ArrayBufferLike> {
  const parts = seed.split(':')
  if (parts.length !== 2) {
    throw new Error(`Invalid seed: ${seed}. Must be in the format <type>:<value>`)
  }
  const type = parts[0].toLowerCase()
  const value = parts[1]

  if (type === 'u8') {
    return Buffer.from([parseInt(value, 10)])
  }
  if (type === 'hex') {
    return Buffer.from(value, 'hex')
  }
  if (type === 'string') {
    return Buffer.from(value)
  }
  if (type === 'pubkey') {
    return new PublicKey(value).toBuffer()
  }
  if (/^[ui](\d+)[lb]e$/.test(type)) {
    const [_, sign, bits, endianness] = type.match(/^([ui])(\d+)([lb]e)$/)!
    const bitLength = parseInt(bits)
    return numberToBuffer(BigInt(value), bitLength, endianness === 'le', sign === 'i')
  }

  throw new Error(
    `Invalid seed type: ${type}. Must be one of: string, pubkey, hex, u8, or matches the pattern [u,i][16,32,64,128][le,be] (for example u64le) for number values`
  )
}

export function numberToBuffer(
  value: bigint,
  bitLength: number,
  littleEndian: boolean,
  signed: boolean
): Buffer {
  const byteLength = bitLength / 8
  const buffer = Buffer.alloc(byteLength)

  const writerMethod =
    bitLength > 48
      ? `writeBig${signed ? '' : 'U'}Int${bitLength.toString(10)}${littleEndian ? 'LE' : 'BE'}`
      : `write${signed ? '' : 'U'}Int${bitLength.toString(10)}${littleEndian ? 'LE' : 'BE'}`
  try {
    if (bitLength > 48) {
      ;(buffer as any)[writerMethod](value)
    } else {
      // cannot use bigint for smaller writer methods, so convert to number
      ;(buffer as any)[writerMethod](parseInt(value.toString(10), 10))
    }
    return buffer
  } catch (err) {
    throw new Error(
      `Failed to write ${bitLength}-bit ${signed ? 'signed' : 'unsigned'} integer to buffer with ${writerMethod} for value ${value}. Make sure the value is within the valid range for the specified bit length and sign. ${err instanceof Error ? err.message : String(err)}`
    )
  }
}

export function bufferToNumber(
  buffer: Buffer,
  bitLength: number,
  littleEndian: boolean,
  signed: boolean
): bigint {
  const readerMethod =
    bitLength > 48
      ? `readBig${signed ? '' : 'U'}Int${bitLength}${littleEndian ? 'LE' : 'BE'}`
      : `read${signed ? '' : 'U'}Int${bitLength}${littleEndian ? 'LE' : 'BE'}`
  try {
    return (buffer as any)[readerMethod](0)
  } catch (err) {
    throw new Error(
      `Failed to read ${bitLength}-bit ${signed ? 'signed' : 'unsigned'} integer from buffer with ${readerMethod} for buffer ${buffer}. Make sure the input is the correct length (${bitLength / 8} bytes) and endianness. ${err instanceof Error ? err.message : String(err)}`
    )
  }
}
