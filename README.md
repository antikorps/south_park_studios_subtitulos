# South Park Studios Subtítulos
Descarga el subtítulo de un capítulo de South Park a partir de su URL en South Park Studios

## Instalación y ejecución
Descarga desde "Releases" el binario para tu sistema operativo y ejecuta pasando la --url del episodio.
```bash
Usage of ./south_park_studios_subtitulos:
  -espera int
        segundos de espera entre la descarga de los segmentos
  -url string
        url del vídeo con el episodio del que se quiere descargar el subtítulo
```
La --espera es opcional, por defecto no es necesaria, no obstante, la automatización de la descarga de numerosos episodios tal vez genere un exceso de peticiones que podría solventarse estableciendo esta espera.