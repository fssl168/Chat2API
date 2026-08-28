/**
 * Agnes Stream Response Handler
 * Converts OpenAI-compatible SSE stream from Agnes gateway
 */

import { PassThrough } from 'stream'
import type { ToolCallingPlan } from '../toolCalling/types'
import { parseToolCallsFromText } from '../utils/toolParser'
import { ToolStreamParser } from '../toolCalling/ToolStreamParser'

const MODEL_NAME = 'agnes'

interface StreamChunk {
  id: string
  model: string
  object: string
  created: number
  choices: Array<{
    index: number
    delta: {
      role?: string
      content?: string
      tool_calls?: any[]
    }
    finish_reason?: string | null
  }>
}

export class AgnesStreamHandler {
  private model: string
  private messageId: string
  private created: number
  private isFirstChunk: boolean = true
  private toolStreamParser?: ToolStreamParser
  private toolCallingPlan?: ToolCallingPlan
  private isDone: boolean = false
  private onEnd?: () => void

  constructor(
    model: string,
    onEnd?: () => void,
    toolCallingPlan?: ToolCallingPlan,
  ) {
    this.model = model
    this.created = Math.floor(Date.now() / 1000)
    this.onEnd = onEnd
    this.toolCallingPlan = toolCallingPlan
    this.toolStreamParser = toolCallingPlan?.shouldParseResponse ? new ToolStreamParser(toolCallingPlan) : undefined
  }

  getLastMessageId(): string {
    return this.messageId
  }

  async handleStream(stream: NodeJS.ReadableStream): Promise<NodeJS.ReadableStream> {
    if (!stream) {
      throw new Error('handleStream: stream is undefined')
    }

    const transStream = new PassThrough()
    let buffer = ''

    stream.on('data', (chunk: Buffer) => {
      buffer += chunk.toString()
      const lines = buffer.split('\n')
      buffer = lines.pop() || ''

      for (const line of lines) {
        if (!line.trim() || !line.startsWith('data:')) continue

        const data = line.slice(5).trim()
        if (data === '[DONE]') {
          this.handleDone(transStream)
          return
        }

        try {
          const parsed = JSON.parse(data) as StreamChunk
          this.processChunk(parsed, transStream)
        } catch (e) {
          console.error('[Agnes] Failed to parse stream chunk:', e)
        }
      }
    })

    stream.on('end', () => {
      this.handleDone(transStream)
    })

    stream.on('error', (err) => {
      transStream.emit('error', err)
    })

    return transStream
  }

  private processChunk(chunk: StreamChunk, transStream: PassThrough): void {
    if (!chunk.choices || chunk.choices.length === 0) return

    const choice = chunk.choices[0]
    if (!choice.delta) return

    if (!this.messageId && chunk.id) {
      this.messageId = chunk.id
    }

    // Handle tool calls
    if (choice.delta.tool_calls && this.toolStreamParser) {
      const baseChunk = this.createBaseChunk()
      const toolChunks = this.toolStreamParser.push(
        JSON.stringify(choice.delta.tool_calls),
        baseChunk,
        this.isFirstChunk,
      )

      for (const tc of toolChunks) {
        transStream.write(`data: ${JSON.stringify(tc)}\n\n`)
        this.isFirstChunk = false
      }

      if (this.toolStreamParser.isBuffering() || this.toolStreamParser.hasEmittedToolCall()) {
        return
      }
    }

    // Handle regular content
    const content = choice.delta.content || ''
    if (!content) return

    const baseChunk = this.createBaseChunk(content)
    transStream.write(`data: ${JSON.stringify(baseChunk)}\n\n`)
    this.isFirstChunk = false
  }

  private createBaseChunk(content?: string) {
    const delta: any = { role: 'assistant' }
    if (content) delta.content = content

    return {
      id: this.messageId || `${MODEL_NAME}@${Date.now()}`,
      model: this.model,
      object: 'chat.completion.chunk',
      created: this.created,
      choices: [{
        index: 0,
        delta,
        finish_reason: null,
      }],
    }
  }

  private handleDone(transStream: PassThrough): void {
    if (this.isDone) return
    this.isDone = true

    // Flush tool call buffer
    const baseChunk = this.createBaseChunk()
    const flushChunks = this.toolStreamParser?.flush(baseChunk) ?? []
    for (const fc of flushChunks) {
      transStream.write(`data: ${JSON.stringify(fc)}\n\n`)
    }

    const finishReason = this.toolStreamParser?.hasEmittedToolCall() ? 'tool_calls' : 'stop'
    transStream.write(`data: ${JSON.stringify(this.createBaseChunk())}\n\n`)
    transStream.write('data: [DONE]\n\n')
    transStream.end()
    this.onEnd?.()
  }

  async handleNonStream(stream: NodeJS.ReadableStream): Promise<any> {
    let buffer = ''

    return new Promise((resolve, reject) => {
      stream.on('data', (chunk: Buffer) => {
        buffer += chunk.toString()
      })

      stream.on('end', () => {
        try {
          const parsed = JSON.parse(buffer)
          resolve(parsed)
        } catch (e) {
          reject(e)
        }
      })

      stream.on('error', reject)
    })
  }
}

export const agnesStreamHandler = {
  AgnesStreamHandler,
}
