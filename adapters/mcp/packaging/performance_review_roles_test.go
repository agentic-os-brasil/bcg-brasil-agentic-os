package packaging

import (
	"bytes"
	"testing"
)

func TestPerformanceReviewBindsRolesBeforeInterview(t *testing.T) {
	for _, namespace := range []string{"", "maestro-product", "maestro-desktop-methods"} {
		var files map[string][]byte
		var err error
		if namespace == "maestro-desktop-methods" {
			files, err = desktopFrontendPayload("0.1.48")
		} else {
			files, err = methodPayload(namespace)
		}
		if err != nil {
			t.Fatal(err)
		}
		body := files["skills/fodais-performance-review/SKILL.md"]
		roles := bytes.Index(body, []byte("## Papéis da avaliação"))
		interview := bytes.Index(body, []byte("## Processo — entrevista em etapas"))
		if roles < 0 || interview < roles {
			t.Errorf("%q: role resolution must precede the interview", namespace)
		}
		for _, required := range []string{
			"{avaliador}", "{entrevistador}", "{nome}",
			"pedido explícito atual e do contexto admitido",
			"Se faltar a identificação de qualquer papel, pergunte antes de iniciar",
			"O owner e o interlocutor atual não são automaticamente",
			"Papéis diferentes podem ser exercidos pela mesma pessoa somente quando isso estiver explícito",
			"Dirija as perguntas ao {avaliador}",
			"sinalizar a {avaliador} antes de fechar a etapa",
			"baseado apenas nos inputs atribuídos a {avaliador}",
		} {
			if !bytes.Contains(body, []byte(required)) {
				t.Errorf("%q: missing role-binding contract %q", namespace, required)
			}
		}
	}
}
