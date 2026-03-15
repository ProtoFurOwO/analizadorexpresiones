package main

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"regexp" // <-- ¡Asegúrate de agregar este!
	"strconv"
	"strings"

	_ "github.com/lib/pq"
)

// --- Definición de Estructuras ---
type QueryRequest struct {
	Query string `json:"query"`
}

type QueryResponse struct {
	QueryID     int      `json:"queryId"` // <-- NUEVO: Para saber qué registro actualizar
	IsValid     bool     `json:"isValid"`
	SyntaxError string   `json:"syntaxError,omitempty"`
	SemanticErr string   `json:"semanticError,omitempty"`
	ValidFields []string `json:"validFields,omitempty"`
	Conditions  []string `json:"conditions,omitempty"`
}

// NUEVA ESTRUCTURA para la petición de actualizar el conteo
type UpdateMatchRequest struct {
	ID           int `json:"id"`
	MatchesCount int `json:"matches_count"`
}

var allowedFields = map[string]bool{
	"age": true, "gender": true, "emotion": true, "glasses": true,
}

// Variable global para la conexión a la base de datos
var db *sql.DB

// --- Analizador ---
func analyzeQuery(query string) QueryResponse {
	response := QueryResponse{
		ValidFields: []string{"age", "gender", "emotion", "glasses"},
	}

	query = strings.TrimSpace(query)

	match, _ := regexp.MatchString(`(?i)^FIND\s+faces\s+WHERE\s+(.+)$`, query)
	if !match {
		if !strings.HasPrefix(strings.ToUpper(query), "FIND") {
			response.SyntaxError = "Error sintáctico: La consulta debe iniciar con 'FIND faces WHERE'"
		} else if !strings.Contains(strings.ToUpper(query), "WHERE") {
			response.SyntaxError = "Error sintáctico: Falta la cláusula 'WHERE'"
		} else {
			response.SyntaxError = "Error sintáctico: Estructura mal formada."
		}
		response.IsValid = false
		return response
	}

	conditionsPart := query[strings.Index(strings.ToUpper(query), "WHERE")+5:]
	conditionsPart = strings.TrimSuffix(strings.TrimSpace(conditionsPart), ";")

	rawConditions := strings.Split(strings.ToUpper(conditionsPart), " AND ")
	var parsedConditions []string

	// Regex para separar limpiamente "campo operador valor"
	reCond := regexp.MustCompile(`(?i)^\s*([a-z]+)\s*(>=|<=|>|<|=)\s*(.+)\s*$`)

	for _, cond := range rawConditions {
		matches := reCond.FindStringSubmatch(cond)
		if len(matches) != 4 {
			response.SyntaxError = fmt.Sprintf("Error sintáctico en la condición: '%s'", cond)
			response.IsValid = false
			return response
		}

		field := strings.ToLower(matches[1])
		operator := matches[2]
		value := strings.TrimSpace(matches[3])

		// 1. Verificar existencia del campo
		if !allowedFields[field] {
			response.SemanticErr = fmt.Sprintf("Error semántico: campo '%s' no permitido", field)
			response.IsValid = false
			return response
		}

		// 2. VERIFICACIÓN DE TIPOS (¡Lo que faltaba!)
		switch field {
		case "age":
			// Intentar convertir el valor a número
			if _, err := strconv.ParseFloat(value, 64); err != nil {
				response.SemanticErr = fmt.Sprintf("Error semántico: '%s' debe ser un número (recibiste: %s)", field, value)
				response.IsValid = false
				return response
			}
		case "gender", "emotion":
			// Debe tener comillas simples o dobles
			isString := (strings.HasPrefix(value, "'") && strings.HasSuffix(value, "'")) ||
				(strings.HasPrefix(value, "\"") && strings.HasSuffix(value, "\""))

			if !isString {
				response.SemanticErr = fmt.Sprintf("Error semántico: el valor de '%s' debe ir entre comillas (recibiste: %s)", field, value)
				response.IsValid = false
				return response
			}

			// Los textos no se pueden comparar con > o <, solo con =
			if operator != "=" {
				response.SemanticErr = fmt.Sprintf("Error semántico: el operador '%s' no se puede usar con texto", operator)
				response.IsValid = false
				return response
			}
		case "glasses":
			// Limpiar comillas por si las pusieron y checar que sea true/false
			valClean := strings.ToLower(strings.ReplaceAll(strings.ReplaceAll(value, "'", ""), "\"", ""))
			if valClean != "true" && valClean != "false" {
				response.SemanticErr = fmt.Sprintf("Error semántico: '%s' solo acepta true o false (recibiste: %s)", field, value)
				response.IsValid = false
				return response
			}
		}

		parsedConditions = append(parsedConditions, strings.ToLower(cond))
	}

	response.IsValid = true
	response.Conditions = parsedConditions
	return response
}

// --- Función para guardar en la BD ---
func guardarEnBD(query string, isValid bool, matchesCount int) int {
	if db == nil {
		log.Println("Advertencia: No hay conexión a la BD.")
		return 0
	}

	var id int
	// El RETURNING id nos permite saber bajo qué número se guardó
	sqlStatement := `INSERT INTO historial_consultas (query_text, is_valid, matches_count) VALUES ($1, $2, $3) RETURNING id`
	err := db.QueryRow(sqlStatement, query, isValid, matchesCount).Scan(&id)

	if err != nil {
		log.Printf("Error al guardar en BD: %v\n", err)
		return 0
	}
	return id
}

// --- Controlador de la API ---
func validateHandler(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Access-Control-Allow-Origin", "*")
	w.Header().Set("Content-Type", "application/json")

	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	var req QueryRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	result := analyzeQuery(req.Query)

	// Guardamos en la BD y obtenemos el ID
	insertedID := guardarEnBD(req.Query, result.IsValid, 0)
	result.QueryID = insertedID // Se lo mandamos al frontend

	json.NewEncoder(w).Encode(result)
}

// NUEVO ENDPOINT: Recibe el conteo final y lo actualiza en Postgres
func updateMatchHandler(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Access-Control-Allow-Origin", "*")
	w.Header().Set("Content-Type", "application/json")

	var req UpdateMatchRequest
	json.NewDecoder(r.Body).Decode(&req)

	if db != nil && req.ID > 0 {
		db.Exec(`UPDATE historial_consultas SET matches_count = $1 WHERE id = $2`, req.MatchesCount, req.ID)
		fmt.Printf("Registro %d actualizado con %d matches uwu\n", req.ID, req.MatchesCount)
	}
	json.NewEncoder(w).Encode(map[string]string{"status": "ok"})
}

func main() {
	var err error

	// Cadena de conexión usando los datos del docker-compose
	connStr := "user=josean password=admin dbname=compiladores_db host=localhost port=5432 sslmode=disable"
	db, err = sql.Open("postgres", connStr)
	if err != nil {
		log.Printf("Error iniciando conexión a BD: %v\n", err)
	} else {
		// Probar la conexión
		err = db.Ping()
		if err != nil {
			log.Printf("No se pudo conectar a Postgres (¿está prendido Docker?): %v\n", err)
			db = nil // Lo ponemos en nil para que no intente guardar si falló
		} else {
			fmt.Println("Conectado a PostgreSQL exitosamente :D")
		}
	}

	// Servir frontend
	fs := http.FileServer(http.Dir("./static"))
	http.Handle("/", fs)

	// Endpoint
	http.HandleFunc("/api/validate", validateHandler)
	http.HandleFunc("/api/update_matches", updateMatchHandler) // <-- AGREGAR ESTO

	fmt.Println("Servidor de Go corriendo en http://localhost:8000 .w.")
	log.Fatal(http.ListenAndServe(":8000", nil))
}
