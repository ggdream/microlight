package chat

import (
	"time"

	"github.com/egoist/mygo/ui"
	"github.com/egoist/mygo/ui/shader"
)

var thinkingFlow = shader.MustCompile(`package main

var Travel float

func ColorEffect(pos vec2, color vec4) vec4 {
	size := max(rectSize(), vec2(1, 1))
	point := pos / size
	// Move the whole color field right; the crest travels with it.
	flow := point.x - Travel
	crest := 0.50 + 0.17*sin(flow*6.2831853) + 0.045*sin(flow*12.566371 + 0.7)
	depth := point.y - crest
	band := fract(flow + depth*0.17) * 5.0
	cyan := vec3(0.0, 0.43, 0.50)
	blue := vec3(0.10, 0.34, 0.71)
	violet := vec3(0.41, 0.24, 0.72)
	amber := vec3(0.71, 0.34, 0.10)
	teal := vec3(0.0, 0.45, 0.36)
	t := smoothstep(0.0, 1.0, fract(band))
	result := mix(teal, cyan, t)
	if band < 1.0 {
		result = mix(cyan, blue, t)
	} else if band < 2.0 {
		result = mix(blue, violet, t)
	} else if band < 3.0 {
		result = mix(violet, amber, t)
	} else if band < 4.0 {
		result = mix(amber, teal, t)
	}
	current := 1.0 - smoothstep(0.04, 0.36, abs(depth))
	result = mix(result*0.83, result, current)
	return vec4(result * color.a, color.a)
}
`)

func flowThinkingText(c *ui.Context, label ui.Element) {
	var travel float32
	if !c.Preferences().ReduceMotion {
		const period = 3200 * time.Millisecond
		travel = float32(c.Now().UnixNano()%int64(period)) / float32(period)
		c.After(time.Second / 30)
	}
	label.Effect(thinkingFlow.Effect(shader.Options{Uniforms: map[string]any{"Travel": travel}}))
}
