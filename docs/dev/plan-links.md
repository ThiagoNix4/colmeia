# PLAN-LINKS — pontos de link nos 4 lados dos cards

Objetivo: cada card (terminal e nota) tem pontos de link em **cima, direita, baixo e esquerda**.
Pode-se puxar um link de **qualquer lado** e soltar em **qualquer lado** de outro card.
O lado escolhido é salvo no workspace. Links sem lado salvo (antigos e os criados pelo
`colmeia link`) escolhem o par de lados mais próximo automaticamente e se reajustam quando
os cards se movem.

Worker único: **Frontend** (são poucos arquivos e eles se tocam).

## Arquivos permitidos
- `electron/types.ts` — `WorkspaceEdge` ganha `sourceHandle?: string`, `targetHandle?: string`
  (opcionais: JSON antigo continua válido, sem migração).
- `src/store.ts` — `connect(source, target, sourceHandle?, targetHandle?)`; guarda os handles
  no edge; `type: 'smart'` em todo edge; serialização inclui os handles quando existirem.
  Dedupe continua por par de cards (um link por par).
- `src/App.tsx` — `connectionMode={ConnectionMode.Loose}`; `edgeTypes={{ smart: SmartEdge }}`;
  `defaultEdgeOptions={{ type: 'smart' }}`; `onConnect` repassa `sourceHandle/targetHandle`;
  ao carregar o workspace, edges recebem `type: 'smart'` e os handles salvos; evento `link`
  vindo do socket cria edge sem handle (auto).
- `src/edges/SmartEdge.tsx` (novo) — edge customizado:
  - Se o edge tem handle salvo, usa a posição daquele lado.
  - Se não tem, calcula com `useInternalNode` o par de lados mais próximo (padrão "floating
    edges" do React Flow: centro a centro, escolhe o lado pela direção dominante dx/dy).
  - Desenha com `getBezierPath` (curva suave), `BaseEdge`, mantém `animated`.
- `src/components/Ports.tsx` (novo) — componente que renderiza os 4 `<Handle type="source">`
  com `id` = `top|right|bottom|left` e `className="port port-<lado>"`.
- `src/nodes/TerminalNode.tsx` e `src/nodes/NoteNode.tsx` — trocar os 2 `Handle` atuais por `<Ports />`.
- `src/styles.css` — visual dos pontos:
  - pontos discretos (pequenos, semitransparentes) por padrão; ao passar o mouse no card
    ficam visíveis e crescem levemente; ao arrastar uma conexão (`.react-flow__handle.connectingfrom`
    / `.connectionindicator`), destacam com a cor de acento e um leve glow;
  - edge com cor de acento suave, `stroke-width` ~2, selecionado mais forte;
  - nada pode cobrir o header (arrastar card) nem a área do terminal; o handle de cima
    não pode atrapalhar o `dragHandle` (`.card-header`).

## Fora de escopo
- `electron/apiServer.ts` e `cli/` não mudam (o `link` do CLI continua sem lado = auto).

## Critério de pronto
- `npx tsc --noEmit -p tsconfig.json` limpo. **Não** rodar `npm run build` nem reiniciar o app.
- Workspace antigo (edges só com id/source/target) carrega sem erro.
- Responder `RESUMO:` curto, por arquivo.
