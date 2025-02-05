package log_test

import (
	"bytes"
	"strings"
	"testing"
	"time"

	"github.com/rs/zerolog"
	"github.com/stretchr/testify/require"

	"pkg.world.dev/world-engine/assert"
	"pkg.world.dev/world-engine/cardinal"
	"pkg.world.dev/world-engine/cardinal/log"
	"pkg.world.dev/world-engine/cardinal/search/filter"
	"pkg.world.dev/world-engine/cardinal/testutils"
	"pkg.world.dev/world-engine/cardinal/types"
	"pkg.world.dev/world-engine/cardinal/types/engine"
)

type SendEnergyTx struct {
	From, To string
	Amount   uint64
}

type SendEnergyTxResult struct{}

type EnergyComp struct {
	value int
}

func (EnergyComp) Name() string {
	return "EnergyComp"
}

func testSystem(wCtx engine.Context) error {
	wCtx.Logger().Log().Msg("test")
	q := cardinal.NewSearch().Entity(filter.Contains(filter.Component[EnergyComp]()))
	err := q.Each(wCtx,
		func(entityId types.EntityID) bool {
			energyPlanet, err := cardinal.GetComponent[EnergyComp](wCtx, entityId)
			if err != nil {
				return false
			}
			energyPlanet.value += 10
			err = cardinal.SetComponent[EnergyComp](wCtx, entityId, energyPlanet)
			return err == nil
		},
	)
	if err != nil {
		panic(err)
	}

	return nil
}

func testSystemWarningTrigger(wCtx engine.Context) error {
	time.Sleep(time.Millisecond * 400)
	return testSystem(wCtx)
}

func TestWorldLogger(t *testing.T) {
	// TODO(scott): this test is extremely brittle.
	//  If there is any additioanl logger added, this test will fail.
	//  We should find a way to make this test more robust.

	// replaces internal Logger with one that logs to the buf variable above.
	var buf bytes.Buffer
	bufLogger := zerolog.New(&buf)
	t.Setenv("CARDINAL_LOG_LEVEL", "debug")

	tf := testutils.NewTestFixture(t, nil, cardinal.WithCustomLogger(bufLogger))
	world := tf.World

	assert.NilError(t, cardinal.RegisterMessage[SendEnergyTx, SendEnergyTxResult](world, "alpha"))
	assert.NilError(t, cardinal.RegisterComponent[EnergyComp](world))
	log.World(&bufLogger, world, zerolog.InfoLevel)
	jsonWorldInfoString := `{
					"level":"info",
					"total_components":2,
					"components":
						[
							{
								"component_id":1,
								"component_name":"SignerComponent"
							},
							{
								"component_id":2,
								"component_name":"EnergyComp"
							}
						],
					"total_systems":2,
					"systems":
						[
							"cardinal.CreatePersonaSystem",
							"cardinal.AuthorizePersonaAddressSystem"
						]
				}
`
	// Split the buffer's contents by newline
	lines := bytes.Split(buf.Bytes(), []byte("\n"))

	require.JSONEq(t, jsonWorldInfoString, string(lines[0]))
	buf.Reset()

	energy, err := world.GetComponentByName(EnergyComp{}.Name())
	assert.NilError(t, err)
	components := []types.ComponentMetadata{energy}
	wCtx := cardinal.NewWorldContext(world)
	err = cardinal.RegisterSystems(world, testSystemWarningTrigger)
	assert.NilError(t, err)

	tf.StartWorld()

	assert.NilError(t, err)
	entityID, err := cardinal.Create(wCtx, EnergyComp{})
	assert.NilError(t, err)
	t.Log(buf.String())
	logStrings := strings.Split(buf.String(), "\n")[2:]
	require.JSONEq(
		t, `
			{
				"level":"debug",
				"archetype_id":0,
				"message":"created"
			}`, logStrings[1],
	)
	require.JSONEq(
		t, `
			{
				"level":"debug",
				"components":[{
				"component_id":2,
					"component_name":"EnergyComp"
				}],
				"entity_id":0,"archetype_id":0
			}`, logStrings[2],
	)

	buf.Reset()

	// test log entity
	archetypeID, err := world.GameStateManager().GetArchIDForComponents(components)
	assert.NilError(t, err)
	log.Entity(&bufLogger, zerolog.DebugLevel, entityID, archetypeID, components)
	jsonEntityInfoString := `
		{
			"level":"debug",
			"components":[
				{
					"component_id":2,
					"component_name":"EnergyComp"
				}],
			"entity_id":0,
			"archetype_id":0
		}`
	require.JSONEq(t, buf.String(), jsonEntityInfoString)

	// Create a system for logging.
	buf.Reset()

	// testing output of logging a tick. Should log the system log and tick start and end strings.
	tf.DoTick()
	logStrings = strings.Split(buf.String(), "\n")[:3]
	// test tick start
	require.JSONEq(
		t, `
			{
				"level":"info",
				"tick":0,
				"message":"Tick started"
			}`, logStrings[0],
	)
	// test if updating component worked
	require.JSONEq(
		t, `
			{
				"level":"debug",
				"entity_id":"0",
				"component_name":"EnergyComp",
				"component_id":2,
				"message":"entity updated",
				"system":"log_test.testSystemWarningTrigger"
			}`, logStrings[2],
	)

	// testing log output for the creation of two entities.
	buf.Reset()
	_, err = cardinal.CreateMany(wCtx, 2, EnergyComp{})
	assert.NilError(t, err)
	entityCreationStrings := strings.Split(buf.String(), "\n")[:2]
	require.JSONEq(
		t, `
			{
				"level":"debug",
				"components":
					[
						{
							"component_id":2,
							"component_name":"EnergyComp"
						}
					],
				"entity_id":1,
				"archetype_id":0
			}`, entityCreationStrings[0],
	)
	require.JSONEq(
		t, `
			{
				"level":"debug",
				"components":
					[
						{
							"component_id":2,
							"component_name":"EnergyComp"
						}
					],
				"entity_id":2,
				"archetype_id":0
			}`, entityCreationStrings[1],
	)
}
