import { useRef, useCallback } from 'react'
import Editor, { type OnMount, type BeforeMount } from '@monaco-editor/react'
import type { editor as MonacoEditor, languages, IDisposable } from 'monaco-editor'
import { useTheme } from '@/components/theme/theme-provider'

interface CodeEditorProps {
  value: string
  onChange: (value: string) => void
  language: 'html' | 'javascript' | 'json'
  height?: string
  placeholder?: string
  variables?: string[]
}

export function CodeEditor({
  value,
  onChange,
  language,
  height = '200px',
  placeholder,
  variables = [],
}: CodeEditorProps) {
  const { theme } = useTheme()
  const editorRef = useRef<MonacoEditor.IStandaloneCodeEditor | null>(null)
  const disposablesRef = useRef<IDisposable[]>([])

  const resolvedTheme =
    theme === 'system'
      ? window.matchMedia('(prefers-color-scheme: dark)').matches
        ? 'vs-dark'
        : 'vs'
      : theme === 'dark'
        ? 'vs-dark'
        : 'vs'

  const handleBeforeMount: BeforeMount = useCallback(
    (monaco) => {
      // Clean up previous completions
      disposablesRef.current.forEach((d) => d.dispose())
      disposablesRef.current = []

      if (variables.length === 0) return

      // Register {{Variable}} completions for all languages we use
      const langs: string[] = ['html', 'javascript', 'json']
      for (const lang of langs) {
        const disposable = monaco.languages.registerCompletionItemProvider(lang, {
          triggerCharacters: ['{'],
          provideCompletionItems(
            model: MonacoEditor.ITextModel,
            position: { lineNumber: number; column: number },
          ): languages.CompletionList {
            const textUntilPosition = model.getValueInRange({
              startLineNumber: position.lineNumber,
              startColumn: 1,
              endLineNumber: position.lineNumber,
              endColumn: position.column,
            })

            // Only suggest after {{ pattern
            if (!textUntilPosition.endsWith('{{') && !textUntilPosition.endsWith('{')) {
              return { suggestions: [] }
            }

            const word = model.getWordUntilPosition(position)
            const range = {
              startLineNumber: position.lineNumber,
              endLineNumber: position.lineNumber,
              startColumn: word.startColumn,
              endColumn: word.endColumn,
            }

            const suggestions: languages.CompletionItem[] = variables.map((name) => ({
              label: `{{${name}}}`,
              kind: monaco.languages.CompletionItemKind.Variable,
              insertText: textUntilPosition.endsWith('{{') ? `${name}}}` : `{${name}}}`,
              detail: 'Template variable',
              range,
            }))

            return { suggestions }
          },
        })
        disposablesRef.current.push(disposable)
      }
    },
    [variables],
  )

  const handleMount: OnMount = useCallback((editor) => {
    editorRef.current = editor
    editor.focus()
  }, [])

  return (
    <div className="rounded-md border overflow-hidden">
      <Editor
        height={height}
        language={language}
        value={value}
        theme={resolvedTheme}
        onChange={(v) => onChange(v ?? '')}
        beforeMount={handleBeforeMount}
        onMount={handleMount}
        options={{
          minimap: { enabled: false },
          lineNumbers: 'on',
          wordWrap: 'on',
          fontSize: 13,
          tabSize: 2,
          scrollBeyondLastLine: false,
          automaticLayout: true,
          padding: { top: 8, bottom: 8 },
          renderLineHighlight: 'none',
          overviewRulerLanes: 0,
          hideCursorInOverviewRuler: true,
          scrollbar: {
            vertical: 'auto',
            horizontal: 'auto',
            verticalScrollbarSize: 8,
            horizontalScrollbarSize: 8,
          },
          placeholder,
        }}
      />
    </div>
  )
}
