package pokeapi

// The structs below mirror the few fields of PokéAPI documents the import reads.
// json.Unmarshal ignores every other field.

// named is the {"name", "url"} reference PokéAPI uses to point at another document.
type named struct {
	Name string `json:"name"`
}

// listResponse is a listing such as GET /move?limit=10000.
type listResponse struct {
	Results []named `json:"results"`
}

// localizedName is one translation of a display name.
type localizedName struct {
	Name     string `json:"name"`
	Language named  `json:"language"`
}

type typeResponse struct {
	Name  string          `json:"name"`
	Names []localizedName `json:"names"`
	// syntaxe : une struct anonyme (sans nom de type), utile quand elle ne sert qu'ici.
	DamageRelations struct {
		DoubleDamageTo []named `json:"double_damage_to"`
		HalfDamageTo   []named `json:"half_damage_to"`
		NoDamageTo     []named `json:"no_damage_to"`
	} `json:"damage_relations"`
}

type moveResponse struct {
	Name        string          `json:"name"`
	Names       []localizedName `json:"names"`
	Type        named           `json:"type"`
	DamageClass named           `json:"damage_class"`
	// pourquoi : un JSON null laisse un int à 0 sans erreur. PokéAPI met null pour « pas
	// pourquoi : de dégâts » / « ne rate jamais » : c'est notre convention, gratuitement.
	Power    int `json:"power"`
	Accuracy int `json:"accuracy"`
	PP       int `json:"pp"`
}

type versionGroupResponse struct {
	Name  string `json:"name"`
	Order int    `json:"order"`
}

type speciesResponse struct {
	Name       string          `json:"name"`
	Names      []localizedName `json:"names"`
	Generation named           `json:"generation"`
	// pourquoi : une espèce a plusieurs formes (deoxys-normal, deoxys-attack…) ; on garde
	// pourquoi : celle marquée is_default, dont le nom n'est pas forcément celui de l'espèce.
	Varieties []struct {
		IsDefault bool  `json:"is_default"`
		Pokemon   named `json:"pokemon"`
	} `json:"varieties"`
}

type pokemonResponse struct {
	Stats []struct {
		BaseStat int   `json:"base_stat"`
		Stat     named `json:"stat"`
	} `json:"stats"`
	// pourquoi : PokéAPI rend les types déjà triés par slot : le premier est le principal.
	Types []struct {
		Type named `json:"type"`
	} `json:"types"`
	Moves []pokemonMove `json:"moves"`
}

// pourquoi : ces deux-là ont un nom (pas anonymes) car selectMovepool les reçoit en
// pourquoi : paramètre, et les tests doivent pouvoir en fabriquer.
type pokemonMove struct {
	Move    named         `json:"move"`
	Details []learnDetail `json:"version_group_details"`
}

// learnDetail is one way a creature learns a move in one version group.
type learnDetail struct {
	Level        int   `json:"level_learned_at"`
	Method       named `json:"move_learn_method"`
	VersionGroup named `json:"version_group"`
}
