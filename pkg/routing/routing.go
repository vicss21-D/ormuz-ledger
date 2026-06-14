package routing

import (
	"math"
)

func CalculateStationPosition(slot, totalStations, maxSensors int) Coordinate {
	largura := math.Ceil(math.Sqrt(float64(maxSensors)))
	altura := math.Ceil(float64(maxSensors) / largura)

	bordaX := largura + 1
	bordaY := altura + 1

	perimetroTop := bordaX
	perimetroRight := bordaY
	perimetroBottom := bordaX
	perimetroLeft := bordaY
	perimetroTotal := perimetroTop + perimetroRight + perimetroBottom + perimetroLeft

	espacamento := perimetroTotal / float64(totalStations)
	posicao := (float64(slot) - 0.5) * espacamento

	var x, y float64

	if posicao <= perimetroTop { // Norte
		x = posicao
		y = 0
	} else if posicao <= perimetroTop+perimetroRight { // Leste
		x = bordaX
		y = posicao - perimetroTop
	} else if posicao <= perimetroTop+perimetroRight+perimetroBottom { // Sul
		x = bordaX - (posicao - (perimetroTop + perimetroRight))
		y = bordaY
	} else { // Oeste
		x = 0
		y = bordaY - (posicao - (perimetroTop + perimetroRight + perimetroBottom))
	}

	return Coordinate{X: x, Y: y}
}

// CalculateSensorPosition converte o ID de um setor/sensor em uma coordenada X,Y no grid de batalha
func CalculateSensorPosition(sectorID int, maxSensors int) Coordinate {
	// Proteção contra IDs nulos
	if sectorID <= 0 {
		sectorID = 1
	}

	// Calcula a largura do grid (ex: maxSensors 25 = grid 5x5)
	largura := int(math.Ceil(math.Sqrt(float64(maxSensors))))
	if largura == 0 {
		largura = 1
	}

	// Mapeia o ID sequencial (1D) para Coordenadas Cartesianas (2D)
	// Exemplo: Sector 6 num grid 5x5 fica na posição X:1, Y:2
	x := float64((sectorID-1)%largura) + 1.0
	y := float64((sectorID-1)/largura) + 1.0

	return Coordinate{X: x, Y: y}
}