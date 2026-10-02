package newsletter

import (
	"crypto/rand"
	"math/big"
	"strings"
)

func randomLocalPart() (string, error) {
	words := make([]string, 0, 3)
	for _, pool := range [][]string{addressAdjectives, addressAnimals, addressPlaces} {
		index, err := rand.Int(rand.Reader, big.NewInt(int64(len(pool))))
		if err != nil {
			return "", err
		}
		words = append(words, pool[index.Int64()])
	}
	return strings.Join(words, "-"), nil
}

var addressAdjectives = strings.Fields(`
quiet bright calm able active airy alert alive alpine amber ample ancient
aquatic arctic aromatic artistic attentive autumnal balanced beautiful blue
bold bouncy brave breezy brisk broad bronze bubbly buoyant candid capable
careful caring casual cheerful clear clever cloudless coastal cobalt colorful
comely comfy compact composed cool coral cordial cosmic cozy creative crisp
curious dainty daring dazzling dear decent deep delicate delightful dependable
devoted different diligent distant downy dreamy dry durable eager early earnest
easy elastic elegant emerald energetic even evergreen excellent fair faithful
familiar fancy fast feathered feathery festive fine firm fitting flexible floral
flowing fluffy focused fond forested fragrant free fresh friendly frosty full
gentle giant glad gleaming glimmering glittering glowing golden good graceful
grand grassy grateful green growing happy hardy harmonious hazel helpful hidden
high hilly hollow homely honest hopeful humble hushed icy ideal imaginative
immense inventive ivory jade jaunty jolly joyful jubilant keen kind kindly
knowing large leafy level light lively lofty lovely loyal lucid lucky lunar
lush magical majestic mellow merry mild minty misty modest moonlit mossy moving
musical natural neat nimble noble notable novel oceanic open orange orderly
original outgoing oval pale pastel patient peaceful pearly perfect playful
pleasant poised polished precious pretty proud pure quick radiant rare ready
reflective relaxed reliable remarkable resilient restful rich rolling rosy
round royal rustic safe sandy sapphire scenic serene shiny short shy silent
silky silver simple sincere skillful sleek slender slim small smart smooth
snowy snug soft solid spacious sparkling splendid sprightly stable starry
steady steep still stony strong subtle sunny swift tall tame tender thoughtful
tidal tidy tiny together tranquil true trusting turquoise twilight unique
upbeat useful vast verdant vibrant violet vivid warm watchful welcoming white
whimsical whole wide wild willing winding wintry wise witty wonderful wooden
wooded youthful zesty agile amicable animated appealing approachable assured
authentic autumn azure beaming beloved beneficial blithe blooming blossoming
boundless brilliant captivating charming civilized classic clean close
comfortable confident considerate constant content convenient copper courteous
crystalline cultured dapper dedicated deft democratic detailed dignified
discerning distinctive diverse dynamic earthy effortless enchanting encouraging
endearing enduring enlightened entertaining ethereal exceptional experienced
expressive fabulous fanciful fascinating favorite fearless fertile flourishing
forgiving fortunate frank generous genuine glacial gracious hearty hospitable
indigo industrious insightful inspiring
lavender learned lemon lilac lithe lovable luminous magnificent marvellous
meaningful meditative meticulous mindful modern multicolored nurturing
observant optimistic organized passionate perceptive persistent personable
plentiful positive practical proactive promising punctual purposeful quaint
receptive refined refreshing resourceful respectful responsible rewarding
rippling rounded sage satisfying seasonal selective sensible sharp shining
shimmering soothing sophisticated spirited springlike spry sterling studious
succinct supportive sustainable tactful talented tasteful timeless tolerant
trustworthy understanding unhurried uplifting valiant versatile vivacious
wholehearted wondrous woolly zestful
`)

// Animal vocabulary is adapted from unique-names-generator; see address-words.LICENSE.
var addressAnimals = strings.Fields(`
badger goose aardvark aardwolf albatross alligator alpaca anaconda angelfish
anglerfish ant anteater antelope antlion ape aphid armadillo asp baboon bandicoot
barnacle barracuda basilisk bass bat bear beaver bee beetle bird bison blackbird
boa boar bobcat bobolink bonobo butterfly buzzard camel capybara cardinal caribou
carp cat caterpillar catfish catshark cattle centipede chameleon cheetah
chickadee chicken chimpanzee chinchilla chipmunk cicada clam clownfish cobra cod
condor coral cougar cow coyote crab crane crawdad crayfish cricket crocodile
crow cuckoo damselfly deer dingo dog dolphin donkey dormouse dove dragonfly duck
eagle earthworm earwig echidna eel egret elephant elk emu ermine falcon ferret
finch firefly fish flamingo fly flyingfish fowl fox frog gamefowl gayal gazelle
gecko gerbil gibbon giraffe goat goldfish gopher gorilla grasshopper grouse guan
guanaco guineafowl gull guppy haddock halibut hamster hare harrier hawk hedgehog
heron herring hippopotamus hornet horse hoverfly hummingbird hyena iguana impala
jackal jaguar jay jellyfish junglefowl kangaroo kingfisher kite kiwi koala koi
krill ladybug lamprey lark lemming lemur leopard limpet lion lizard llama lobster
locust loon lungfish lynx macaw mackerel magpie manatee mandrill marlin marmoset
marmot marten meadowlark meerkat mink minnow mockingbird mole mongoose monkey
moose moth mouse mule muskox narwhal newt nightingale ocelot octopus opossum
orangutan orca ostrich otter owl ox panda panther parakeet parrot parrotfish
partridge peacock peafowl pelican penguin perch pheasant pig pigeon pike piranha
platypus pony porcupine porpoise possum prawn ptarmigan puffin puma python quail
quelea quokka rabbit raccoon rat rattlesnake raven reindeer rhinoceros roadrunner
rook rooster sailfish salamander salmon sawfish scallop scorpion seahorse shark
sheep shrew shrimp silkworm silverfish skink skunk sloth slug smelt snail snake
snipe sole sparrow spider spoonbill squid squirrel starfish stingray stoat stork
sturgeon swallow swan swift swordfish swordtail tahr takin tapir tarantula
tarsier termite tern thrush tiger toad tortoise toucan trout tuna turkey turtle
urial vicuna viper vole vulture wallaby walrus warbler wasp weasel whale whippet
whitefish wildcat wildebeest wildfowl wolf wolverine wombat woodpecker worm wren
yak zebra
`)

var addressPlaces = strings.Fields(`
meadow harbor abbey acre alcove alley amphitheater anchorage apartment aqueduct
arcade arch archipelago arena armory atelier atrium attic avenue backyard
balcony bank barn basin bay beach beacon bench bend berth boardwalk boathouse
bog bower branch breakwater bridge brook bungalow burrow busway butte cabin
cafe canal canyon cape campground campus carport cascade castle causeway cave
cavern cellar chalet channel chapel chasm chateau clearing cliff cloister coast
colonnade common conservatory courtyard cove creek crest crossing dale dam dell
delta depot desert dock dune dwelling embankment estuary farm farmland farmstead
fen field fjord flat foothill ford forest fountain foyer gallery garden gate
gateway gazebo glade glen grotto grove gully hamlet haven heath hedge highland
hill hillside hilltop hollow homestead horizon hostel hotel house inlet inn
island islet jetty junction knoll lagoon lake lakeside landing lane lawn ledge
library lighthouse lodge loft lookout lowland mall manor marsh market mill
moor mound mountain museum nook nursery oasis observatory orchard outpost
overlook paddock pagoda palace park pasture path patio pavilion peak peninsula
piazza pier plain plateau plaza pond pool porch prairie promenade quay ravine
reef reservoir retreat ridge river riverside road rock rooftop room rotunda
roundabout route runway sanctuary sandbank school sea seafront seashore shelter
shore sidewalk slope sound spring square stable station step steppe store
storehouse stream street studio summit sundial sunroom swamp terrace theater
thicket tidalflat tower town track trail treehouse trench tunnel upland valley
veranda viaduct village vineyard vista walkway warehouse waterfall watermill
watershed waterfront waterway well wetland wharf woodland workshop yard
airfield airport arbor arboretum auditorium backwater bakery ballroom bandstand
barrow bathhouse bazaar beachside beltway boatharbor bookstore boulevard bridgeway
building cabana campsite canopy canteen carriageway cartway cataract chamber
chaparral city classroom club clubhouse cobblestone coffeeshop concourse cottage
country countryside crag crater crescent croft crossroads culvert cupola desertland
doorway drawingroom drive driveway drumlin earthwork eyrie fairground fell
floodplain footbridge footpath forecourt foreshore funfair garage greenhouse
headland hedgerow hinterland hive icefield
kitchen lab labyrinth laneway livingroom lobby lock
longhouse marketplace mesa millpond monument mudflat ocean office
passage passageway pergola pinewood playfield playground
playhouse port portico precinct promontory
railway ranch readingroom reedbed rise rockpool rosebed sandbar sandhill schoolyard
seawall shop shoreline skiway slipway snowfield spillway stairway stairwell
stockroom strait summerhouse tableland tearoom timberland
townhouse townland towpath tramway treeline underpass vestibule villagegreen
watercourse waterhole wayside weir wildwood windmill woodlot workroom
`)
