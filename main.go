package main

import (
	"bufio"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"time"
	"unicode"
)

type TopazJSONMin struct {
	Stitchedstream struct {
		Source string `json:"source"`
	} `json:"stitchedstream"`
}

func incorporarCabeceras(req *http.Request) {
	req.Header.Set("User-Agent", "Mozilla/5.0 (X11; Ubuntu; Linux x86_64; rv:122.0) Gecko/20100101 Firefox/122.0")
	req.Header.Set("Accept", "text/html,application/xhtml+xml,application/xml;q=0.9,image/avif,image/webp,*/*;q=0.8")
	req.Header.Set("Accept-Language", "es-ES,es;q=0.8,en-US;q=0.5,en;q=0.3")
	req.Header.Set("Upgrade-Insecure-Requests", "1")
	req.Header.Set("Connection", "keep-alive")
	req.Header.Set("Sec-Fetch-Dest", "document")
	req.Header.Set("Sec-Fetch-Mode", "navigate")
	req.Header.Set("Sec-Fetch-Site", "none")
	req.Header.Set("Sec-Fetch-User", "?1")
	req.Header.Set("Pragma", "no-cache")
	req.Header.Set("Cache-Control", "no-cache")
	req.Header.Set("TE", "trailers")
}

/*
	Los segmentos devuelven líneas que tal vez aparecieron en el segmento anterior.
	Mediante el indicador temporal 00:00:03.803 --> 00:00:05.939
	se asegura que esa marca temporal solo hay uno y se evitan duplicados
*/
func eliminarLineasTemporadlesDuplicadasVtt(contenido string) string {
	// Mapara para incorporar indicadores temporales utilizados
	indicadoresUtilizados := make(map[string]string)

	// Los archivos vtt deben comenzar así
	vttSinDuplicidades := "WEBVTT\n\n"

	// Recorrer contenido línea a línea
	escaner := bufio.NewScanner(strings.NewReader(contenido))
	escaner.Split(bufio.ScanLines)

	incorporar := false
	for escaner.Scan() {
		linea := escaner.Text()
		if strings.Contains(linea, " --> ") {
			// Se trata de una línea de indicador temporal
			indicadorTemporal := strings.Split(linea, " --> ")
			indicadorInicio := indicadorTemporal[0]
			_, existe := indicadoresUtilizados[indicadorInicio]
			if existe {
				// Existe en el mapa, se ha utilizado, no incorporar nada hasta comprobar la próxima línea con indicador temporal
				incorporar = false
				continue
			}

			// No existe en el mapa, incorporar y seguir incorporando hasta comprobar la próxima línea con indicador temporal
			indicadoresUtilizados[indicadorInicio] = "s"
			vttSinDuplicidades += linea + "\n"
			incorporar = true
			continue
		}
		// Línea de contenido, incorporar en caso necesario
		if incorporar {
			vttSinDuplicidades += linea + "\n"
		}
	}

	return vttSinDuplicidades
}

func main() {
	// parsear url y espera (por si llegara a devolver un status 429)
	var url string
	var espera int
	flag.StringVar(&url, "url", "", "url del vídeo con el episodio del que se quiere descargar el subtítulo")
	flag.IntVar(&espera, "espera", 0, "segundos de espera entre la descarga de los segmentos")

	flag.Parse()

	if url == "" {
		log.Fatalln("ERROR CRÍTICO: no se ha proporcionado ninguna url y se esperaba una del tipo https://www.southparkstudios.com/episodes/4yl119/south-park-band-in-china-season-23-ep-2")
	}

	// recuperar rutas
	ejecutableRuta, ejecutableRutaError := os.Executable()
	if ejecutableRutaError != nil {
		log.Fatalln("ERROR CRÍTICO: no se ha podido recuperar la ruta del ejecutable")
	}
	directorioRuta := filepath.Dir(ejecutableRuta)

	// Cliente http. Las respuestas son ligeras, un timeout de 5 segundos debería ser más que suficinete
	cliente := http.Client{
		Timeout: time.Duration(5 * time.Second),
	}

	// Petición a la URL del vídeo. Se espera obtener videoServiceUrl y el código del episodio para nombrar el archivo
	videoPeticion, videoPeticionError := http.NewRequest("GET", url, nil)
	if videoPeticionError != nil {
		log.Fatalln("ERROR CRÍTICO: fallo en la preparación de la petición del vídeo", videoPeticionError)
	}
	incorporarCabeceras(videoPeticion)

	videoRespuesta, videoRespuestaError := cliente.Do(videoPeticion)
	if videoRespuestaError != nil {
		log.Fatalln("ERROR CRÍTICO: fallo en la respuesta de la petición del vídeo", videoRespuestaError)
	}
	defer videoRespuesta.Body.Close()
	if videoRespuesta.StatusCode != 200 {
		log.Fatalln("ERROR CRÍTICO: la petición del vídeo ha tenido un status code incorrecto", videoRespuesta.Status)
	}

	videoHtmlBytes, videoHtmlBytesError := io.ReadAll(videoRespuesta.Body)
	if videoHtmlBytesError != nil {
		log.Fatalln("ERROR CRÍTICO: no se ha podido leer el contenido del cuerpo de la petición del vídeo", videoHtmlBytesError)
	}

	// Buscar videoServiceUrl
	videoHtml := strings.ReplaceAll(string(videoHtmlBytes), "\n", "")
	videoServiceExpReg := regexp.MustCompile(`"videoServiceUrl":"(.*?)"`)

	videoServiceCoincidencias := videoServiceExpReg.FindStringSubmatch(videoHtml)
	if len(videoServiceCoincidencias) != 2 {
		log.Fatalln("ERROR CRÍTICO: no se ha podido localizar el videoServiceUrl, ha fallado la expresión regular con un número de coincidencias distinto a 2", len(videoServiceCoincidencias), "asegurar que la URL proporcionada es parecida a https://www.southparkstudios.com/episodes/4yl119/south-park-band-in-china-season-23-ep-2")
	}

	videoServiceUrl := videoServiceCoincidencias[1]
	videoServiceIdentifdicadorExpReg := regexp.MustCompile(`shared\.southpark\.global:(.*?)\\`)
	videoServiceIdentificadorCoincidencias := videoServiceIdentifdicadorExpReg.FindStringSubmatch(videoServiceUrl)
	if len(videoServiceIdentificadorCoincidencias) != 2 {
		log.Fatalln("ERROR CRÍTICO: no se ha podido localizar el identificador del videoServiceUrl, ha fallado la expresión regular con un número de coincidencias distinto a 2", len(videoServiceIdentificadorCoincidencias))
	}
	videoServiceIdentificador := videoServiceIdentificadorCoincidencias[1]

	// Buscar el código del episodio para nombrar el archivo
	nombreArchivo := "south_park_subtitulo.vtt"
	codigoEpisoExpReg := regexp.MustCompile(`"episodeNumber":"(.*?)","seasonNumber"`)
	codigoEpisodioCoincidencias := codigoEpisoExpReg.FindStringSubmatch(videoHtml)
	if len(codigoEpisodioCoincidencias) == 2 {
		nombreArchivo = fmt.Sprintf("south_park_%v.vtt", codigoEpisodioCoincidencias[1])
	}

	vttRuta := filepath.Join(directorioRuta, nombreArchivo)

	// Petición a Topaz donde se espera encontrar el source del episodio
	topazUrl := fmt.Sprintf("https://topaz.viacomcbs.digital/topaz/api/mgid:arc:episode:shared.southpark.global:%v/mica.json?clientPlatform=mobile", videoServiceIdentificador)

	topazPeticion, topazPeticionError := http.NewRequest("GET", topazUrl, nil)
	if topazPeticionError != nil {
		log.Fatalln("ERROR CRÍTICO: fallo en la preparación de la petición a Topaz", topazPeticionError)
	}
	incorporarCabeceras(topazPeticion)
	topazRespuesta, topazRespuestaError := cliente.Do(topazPeticion)
	if topazRespuestaError != nil {
		log.Fatalln("ERROR CRÍTICO: fallo en la respuesta de la petición a Topaz", topazRespuestaError)
	}
	defer topazRespuesta.Body.Close()
	if topazRespuesta.StatusCode != 200 {
		log.Fatalln("ERROR CRÍTICO: la petición a ha tenido un status code incorrecto", topazRespuesta.Status)
	}

	var topazJSONMin TopazJSONMin
	topazDeserializacionError := json.NewDecoder(topazRespuesta.Body).Decode(&topazJSONMin)
	if topazDeserializacionError != nil {
		log.Fatalln("ERROR CRÍTICO: no se ha podido deserializar la respuesta de Topaz necesaria para obtener el source", topazDeserializacionError)
	}
	if topazJSONMin.Stitchedstream.Source == "" {
		log.Fatalln("ERROR CRÍTICO: la deserialización de la respuesta a Topaz ha devuelto un source vacío")
	}

	// Buscar source del episodio
	sourceExpReg := regexp.MustCompile(`topaz\.viacomcbs\.digital/h/a/(.*?)/`)
	sourceCoincidencias := sourceExpReg.FindStringSubmatch(topazJSONMin.Stitchedstream.Source)
	if len(sourceCoincidencias) != 2 {
		log.Fatalln("ERROR CRÍTICO: no se ha podido localizar el identificador del source en Topaz, ha fallado la expresión regular con un número de coincidencias distinto a 2", len(sourceCoincidencias))
	}
	source := sourceCoincidencias[1]

	// Recuperar el contenido del vtt incrementando segmentos
	var vtt string

	contador := 0
iterarSegmentos:
	for {
		contador++

		time.Sleep(time.Duration(espera * int(time.Second)))
		fmt.Printf("\rDESCARGANDO SEGMENTO: %d", contador)
		vttUrl := fmt.Sprintf("https://southpark.orchestrator.viacomcbs-tech.com/h/a/%v/segment-%d.vtt", source, contador)

		segmentoPeticion, segmentoPeticionError := http.NewRequest("GET", vttUrl, nil)
		if segmentoPeticionError != nil {
			log.Fatalln("ERROR CRÍTICO: ha fallado la preparación de la petición en el segmento", contador, segmentoPeticionError)
		}
		incorporarCabeceras(segmentoPeticion)

		segmentoRespuesta, segmentoRespuestaError := cliente.Do(segmentoPeticion)
		if segmentoRespuestaError != nil {
			log.Fatalln("ERROR CRÍTICO: ha fallado la respuesta en el segmento", contador, segmentoRespuesta)
		}
		defer segmentoRespuesta.Body.Close()
		if segmentoRespuesta.StatusCode != 200 {
			if segmentoRespuesta.StatusCode == 400 {
				break iterarSegmentos
			} else {
				log.Fatalln("ERROR CRÍTICO: status code incorrecto en la respuesta del segmento", contador, segmentoRespuesta.Status)
			}
		}

		vttBytes, vttBytesError := io.ReadAll(segmentoRespuesta.Body)
		if vttBytesError != nil {
			log.Fatalln("ERROR CRÍTICO: fallo al recuperar el contenido de la respuesta del segmento", contador, vttBytes)
		}

		// Eliminar la cabecera que se repite en cada segmento, comenzar a incorporar desde marca temporal
		var vttContenidoDepurado string
		vttContenido := string(vttBytes)
		incorporar := false
		for _, v := range vttContenido {
			if unicode.IsDigit(v) {
				incorporar = true
			}
			if incorporar {
				vttContenidoDepurado += string(v)
			}
		}

		vtt += vttContenidoDepurado
	}
	fmt.Println("")

	// Corregir duplicidades
	vtt = eliminarLineasTemporadlesDuplicadasVtt(vtt)

	vttArchivo, vttArchivoError := os.Create(vttRuta)
	if vttArchivoError != nil {
		log.Println(vtt)
		log.Fatalln("ERROR CRÍTICO: ha fallado la creación del archivo para guardar el vtt", vttArchivoError, "El contenido del archivo se ha pintado en la stdout")
	}
	_, escrituraVttError := vttArchivo.WriteString(vtt)
	if escrituraVttError != nil {
		log.Println(vtt)
		log.Fatalln("ERROR CRÍTICO: ha fallado la escritura del contenido del archivo vtt", escrituraVttError, "El contenido del archivo se ha pintado en la stdout")
	}

	log.Println("ÉXITO: subtitulo descargado correctamente y disponible en", vttRuta)
}
