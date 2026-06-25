# ESPManager

Gerenciador open source, leve e self-hosted (homelab/Proxmox) de frotas de dispositivos ESP32, com deploy de firmware por OTA disparado por `git push`. Backend em Go (binário único), MQTT embutido, SQLite, UI HTMX.

## Regras de desenvolvimento

Obrigatórias para todas as sessões e todos os agentes que trabalharem neste projeto.

### Código
- Aplicar **SOLID com rigor**: responsabilidade única, aberto/fechado, substituição de Liskov, segregação de interfaces, inversão de dependência (depender de abstrações, não de implementações). Código limpo e desacoplado.
- **Sem comentários desnecessários.** O código deve ser autoexplicativo (nomes claros, funções pequenas). Comentar apenas o "porquê" não óbvio — nunca o "o quê" óbvio nem comentários redundantes.

### Commits
- Autor: email `hello@leohammes.dev`.
- **Nunca** incluir `Co-Authored-By` nem qualquer atribuição a Claude / Claude Code.
- Mensagens limpas, descrevendo **apenas a funcionalidade**.
- **Proibido** referenciar metadados de processo/plano (ex.: "fase 1", "fase 2", "MVP", nomes de etapas internas).

### Documentação
- Não criar documentos no repositório (ROADMAP, ARCHITECTURE, etc.) por enquanto. A documentação será escrita quando o projeto estiver pronto.

### Processo
- **Nunca desenvolver direto na `main`.** Todo trabalho acontece em uma branch (`feature/...`, `fix/...`).
- **Antes de integrar na `main`, rodar um review de código** (workflow multi-agente, automático) e corrigir os achados; só então fazer merge. Não é necessário pedir permissão a cada vez.
- Usar **workflows** para tarefas substanciais, visando qualidade e velocidade.
