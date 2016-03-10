// policyEngine.go
package policy

import (
	"reflect"
	"sort"
	"strconv"
	"strings"
	"utils/netUtils"
	"utils/patriciaDB"
	"utils/policy/policyCommonDefs"
	//	"utils/commonDefs"
	//	"net"
	//	"asicdServices"
	//	"asicd/asicdConstDefs"
	"bytes"
	//  "database/sql"
)

func (db *PolicyEngineDB) ActionListHasAction(actionList []PolicyAction, actionType int, action string) (match bool) {
	db.Logger.Println("ActionListHasAction for action ", action)
	return match
}

func (db *PolicyEngineDB) ActionNameListHasAction(actionList []string, actionType int, action string) (match bool) {
	db.Logger.Println("ActionListHasAction for action ", action)
	return match
}

func (db *PolicyEngineDB) PolicyEngineCheckActionsForEntity(entity PolicyEngineFilterEntityParams, policyConditionType int) (actionList []string) {
	db.Logger.Println("PolicyEngineTest to see if there are any policies for condition ", policyConditionType)
	var policyStmtList []string
	switch policyConditionType {
	case policyCommonDefs.PolicyConditionTypeDstIpPrefixMatch:
		break
	case policyCommonDefs.PolicyConditionTypeProtocolMatch:
		policyStmtList = db.ProtocolPolicyListDB[entity.RouteProtocol]
		break
	default:
		db.Logger.Println("Unknown conditonType")
		return nil
	}
	if policyStmtList == nil || len(policyStmtList) == 0 {
		db.Logger.Println("no policy statements configured for this protocol")
		return nil
	}
	for i := 0; i < len(policyStmtList); i++ {
		db.Logger.Println("Found policy stmt ", policyStmtList[i], " for this entity")
		policyList := db.PolicyStmtPolicyMapDB[policyStmtList[i]]
		if policyList == nil || len(policyList) == 0 {
			db.Logger.Println("No policies configured for this entity")
			return nil
		}
		for j := 0; j < len(policyList); j++ {
			db.Logger.Println("Found policy ", policyList[j], "for this statement")
			policyStmtInfo := db.PolicyStmtDB.Get(patriciaDB.Prefix(policyStmtList[i]))
			if policyStmtInfo == nil {
				db.Logger.Println("Did not find this stmt in the DB")
				return nil
			}
			policyStmt := policyStmtInfo.(PolicyStmt)
			if db.ConditionCheckValid(entity, policyStmt.Conditions, policyStmt) {
				db.Logger.Println("All conditions valid for this route, so this policy will be potentially applied to this route")
				return policyStmt.Actions
			}
		}
	}
	return actionList
}
func (db *PolicyEngineDB) PolicyEngineUndoActionsPolicyStmt(policy Policy, policyStmt PolicyStmt, params interface{}, conditionsAndActionsList ConditionsAndActionsList) {
	db.Logger.Println("policyEngineUndoActionsPolicyStmt")
	if conditionsAndActionsList.ActionList == nil {
		db.Logger.Println("No actions")
		return
	}
	var i int
	conditionInfoList := make([]interface{}, 0)
	for j := 0; j < len(conditionsAndActionsList.ConditionList); j++ {
		conditionInfoList = append(conditionInfoList, conditionsAndActionsList.ConditionList[j].ConditionInfo)
	}

	for i = 0; i < len(conditionsAndActionsList.ActionList); i++ {
		db.Logger.Printf("Find policy action number %d name %s in the action database\n", i, conditionsAndActionsList.ActionList[i])
		/*
			actionItem := db.PolicyActionsDB.Get(patriciaDB.Prefix(policyStmt.Actions[i]))
			if actionItem == nil {
				db.Logger.Println("Did not find action ", conditionsAndActionsList.ActionList[i], " in the action database")
				continue
			}
			actionInfo := actionItem.(PolicyAction)
		*/
		policyAction := conditionsAndActionsList.ActionList[i]
		if db.UndoActionfuncMap[policyAction.ActionType] != nil {
			db.UndoActionfuncMap[policyAction.ActionType](policyAction.ActionInfo, conditionInfoList, params, policyStmt)
		}
	}
}
func (db *PolicyEngineDB) PolicyEngineUndoPolicyForEntity(entity PolicyEngineFilterEntityParams, policy Policy, params interface{}) {
	db.Logger.Println("policyEngineUndoPolicyForRoute - policy name ", policy.Name, "  route: ", entity.DestNetIp, " type:", entity.RouteProtocol)
	if db.GetPolicyEntityMapIndex == nil {
		return
	}
	policyEntityIndex := db.GetPolicyEntityMapIndex(entity, policy.Name)
	if policyEntityIndex == nil {
		db.Logger.Println("policy entity map index nil")
		return
	}
	policyStmtMap := db.PolicyEntityMap[policyEntityIndex]
	if policyStmtMap.PolicyStmtMap == nil {
		db.Logger.Println("Unexpected:None of the policy statements of this policy have been applied on this route")
		return
	}
	for stmt, conditionsAndActionsList := range policyStmtMap.PolicyStmtMap {
		db.Logger.Println("Applied policyStmtName ", stmt)
		policyStmt := db.PolicyStmtDB.Get(patriciaDB.Prefix(stmt))
		if policyStmt == nil {
			db.Logger.Println("Invalid policyStmt")
			continue
		}
		db.PolicyEngineUndoActionsPolicyStmt(policy, policyStmt.(PolicyStmt), params, conditionsAndActionsList)
		//check if the route still exists - it may have been deleted by the previous statement action
		if db.IsEntityPresentFunc != nil {
			if !(db.IsEntityPresentFunc(params)) {
				db.Logger.Println("This entity no longer exists")
				break
			}
		}
	}
}
func (db *PolicyEngineDB) PolicyEngineImplementActions(entity PolicyEngineFilterEntityParams, policyStmt PolicyStmt,
	conditionInfoList []interface{}, params interface{}) (policActionList []PolicyAction) {
	db.Logger.Println("policyEngineImplementActions")
	policActionList = make([]PolicyAction, 0)
	if policyStmt.Actions == nil {
		db.Logger.Println("No actions")
		return policActionList
	}
	var i int
	addActionToList := false
	for i = 0; i < len(policyStmt.Actions); i++ {
		addActionToList = false
		db.Logger.Printf("Find policy action number %d name %s in the action database\n", i, policyStmt.Actions[i])
		actionItem := db.PolicyActionsDB.Get(patriciaDB.Prefix(policyStmt.Actions[i]))
		if actionItem == nil {
			db.Logger.Println("Did not find action ", policyStmt.Actions[i], " in the action database")
			continue
		}
		action := actionItem.(PolicyAction)
		db.Logger.Printf("policy action number %d type %d\n", i, action.ActionType)
		switch action.ActionType {
		/*
			case policyCommonDefs.PolicyActionTypeRouteDisposition:
				db.Logger.Println("PolicyActionTypeRouteDisposition action to be applied")
				addActionToList = true
				if db.ActionfuncMap[policyCommonDefs.PolicyActionTypeRouteDisposition] != nil {
					db.ActionfuncMap[policyCommonDefs.PolicyActionTypeRouteDisposition](action.ActionInfo, conditionList, params)
				}
				break
			case policyCommonDefs.PolicyActionTypeRouteRedistribute:
				db.Logger.Println("PolicyActionTypeRouteRedistribute action to be applied")
				if db.ActionfuncMap[policyCommonDefs.PolicyActionTypeRouteRedistribute] != nil {
					db.ActionfuncMap[policyCommonDefs.PolicyActionTypeRouteRedistribute](action.ActionInfo, conditionList, params)
				}
				addActionToList = true
				break
			case policyCommonDefs.PolicyActionTypeNetworkStatementAdvertise:
				db.Logger.Println("PolicyActionTypeNetworkStatementAdvertise action to be applied")
				if db.ActionfuncMap[policyCommonDefs.PolicyActionTypeNetworkStatementAdvertise] != nil {
					db.ActionfuncMap[policyCommonDefs.PolicyActionTypeNetworkStatementAdvertise](action.ActionInfo, conditionList, params)
				}
				addActionToList = true
				break
			case policyCommonDefs.PolicyActionTypeAggregate:
				db.Logger.Println("PolicyActionTypeAggregate action to be applied")
				if db.ActionfuncMap[policyCommonDefs.PolicyActionTypeAggregate] != nil {
					db.ActionfuncMap[policyCommonDefs.PolicyActionTypeAggregate](action.ActionInfo, conditionList, params)
				}
				addActionToList = true
				break
		*/
		case policyCommonDefs.PolicyActionTypeRouteDisposition, policyCommonDefs.PolicyActionTypeRouteRedistribute,
			policyCommonDefs.PolicyActionTypeNetworkStatementAdvertise, policyCommonDefs.PolicyActionTypeAggregate:
			db.Logger.Println("action to be applied", action.ActionType)
			if db.ActionfuncMap[action.ActionType] != nil {
				db.ActionfuncMap[action.ActionType](action.ActionInfo, conditionInfoList, params)
			}
			addActionToList = true
		default:
			db.Logger.Println("UnknownInvalid type of action")
			break
		}
		if addActionToList == true {
			policActionList = append(policActionList, action)
		}
	}
	return policActionList
}

func (db *PolicyEngineDB) FindPrefixMatch(ipAddr string, ipPrefix patriciaDB.Prefix, policyName string) (match bool) {
	db.Logger.Println("Prefix match policy ", policyName)
	policyListItem := db.PrefixPolicyListDB.GetLongestPrefixNode(ipPrefix)
	if policyListItem == nil {
		db.Logger.Println("intf stored at prefix ", ipPrefix, " is nil")
		return false
	}
	if policyListItem != nil && reflect.TypeOf(policyListItem).Kind() != reflect.Slice {
		db.Logger.Println("Incorrect data type for this prefix ")
		return false
	}
	policyListSlice := reflect.ValueOf(policyListItem)
	for idx := 0; idx < policyListSlice.Len(); idx++ {
		prefixPolicyListInfo := policyListSlice.Index(idx).Interface().(PrefixPolicyListInfo)
		if prefixPolicyListInfo.policyName != policyName {
			db.Logger.Println("Found a potential match for this prefix but the policy ", policyName, " is not what we are looking for")
			continue
		}
		if prefixPolicyListInfo.lowRange == -1 && prefixPolicyListInfo.highRange == -1 {
			db.Logger.Println("Looking for exact match condition for prefix ", prefixPolicyListInfo.ipPrefix)
			if bytes.Equal(ipPrefix, prefixPolicyListInfo.ipPrefix) {
				db.Logger.Println(" Matched the prefix")
				return true
			} else {
				db.Logger.Println(" Did not match the exact prefix")
				return false
			}
		}
		tempSlice := strings.Split(ipAddr, "/")
		maskLen, err := strconv.Atoi(tempSlice[1])
		if err != nil {
			db.Logger.Println("err getting maskLen")
			return false
		}
		db.Logger.Println("Mask len = ", maskLen)
		if maskLen < prefixPolicyListInfo.lowRange || maskLen > prefixPolicyListInfo.highRange {
			db.Logger.Println("Mask range of the route ", maskLen, " not within the required mask range:", prefixPolicyListInfo.lowRange, "..", prefixPolicyListInfo.highRange)
			return false
		} else {
			db.Logger.Println("Mask range of the route ", maskLen, " within the required mask range:", prefixPolicyListInfo.lowRange, "..", prefixPolicyListInfo.highRange)
			return true
		}
	}
	return match
}
func (db *PolicyEngineDB) DstIpPrefixMatchConditionfunc(entity PolicyEngineFilterEntityParams, condition PolicyCondition, policyStmt PolicyStmt) (match bool) {
	db.Logger.Println("dstIpPrefixMatchConditionfunc")
	ipPrefix, err := netUtils.GetNetworkPrefixFromCIDR(entity.DestNetIp)
	if err != nil {
		db.Logger.Println("Invalid ipPrefix for the route ", entity.DestNetIp)
		return false
	}
	match = db.FindPrefixMatch(entity.DestNetIp, ipPrefix, policyStmt.Name)
	if match {
		db.Logger.Println("Found a match for this prefix")
	}
	return match
}
func (db *PolicyEngineDB) ProtocolMatchConditionfunc(entity PolicyEngineFilterEntityParams, condition PolicyCondition, policyStmt PolicyStmt) (match bool) {
	db.Logger.Println("protocolMatchConditionfunc")
	matchProto := condition.ConditionInfo.(string)
	if matchProto == entity.RouteProtocol {
		db.Logger.Println("Protocol condition matches")
		match = true
	}
	return match
}
func (db *PolicyEngineDB) ConditionCheckValid(entity PolicyEngineFilterEntityParams, conditionsList []string, policyStmt PolicyStmt) (valid bool) {
	db.Logger.Println("conditionCheckValid")
	valid = true
	if conditionsList == nil {
		db.Logger.Println("No conditions to match, so valid")
		return true
	}
	for i := 0; i < len(conditionsList); i++ {
		db.Logger.Printf("Find policy condition number %d name %s in the condition database\n", i, policyStmt.Conditions[i])
		conditionItem := db.PolicyConditionsDB.Get(patriciaDB.Prefix(conditionsList[i]))
		if conditionItem == nil {
			db.Logger.Println("Did not find condition ", conditionsList[i], " in the condition database")
			continue
		}
		condition := conditionItem.(PolicyCondition)
		db.Logger.Printf("policy condition number %d type %d\n", i, condition.ConditionType)
		if db.ConditionCheckfuncMap[condition.ConditionType] != nil {
			match := db.ConditionCheckfuncMap[condition.ConditionType](entity, condition, policyStmt)
			if !match {
				db.Logger.Println("Condition does not match")
				return false
			}
		}
	}
	db.Logger.Println("returning valid= ", valid)
	return valid
}
func (db *PolicyEngineDB) PolicyEngineMatchConditions(entity PolicyEngineFilterEntityParams, policyStmt PolicyStmt) (match bool, conditionsList []PolicyCondition) {
	db.Logger.Println("policyEngineMatchConditions")
	var i int
	allConditionsMatch := true
	anyConditionsMatch := false
	addConditiontoList := false
	conditionsList = make([]PolicyCondition, 0)
	for i = 0; i < len(policyStmt.Conditions); i++ {
		addConditiontoList = false
		db.Logger.Printf("Find policy condition number %d name %s in the condition database\n", i, policyStmt.Conditions[i])
		conditionItem := db.PolicyConditionsDB.Get(patriciaDB.Prefix(policyStmt.Conditions[i]))
		if conditionItem == nil {
			db.Logger.Println("Did not find condition ", policyStmt.Conditions[i], " in the condition database")
			continue
		}
		condition := conditionItem.(PolicyCondition)
		db.Logger.Printf("policy condition number %d type %d\n", i, condition.ConditionType)
		if db.ConditionCheckfuncMap[condition.ConditionType] != nil {
			match = db.ConditionCheckfuncMap[condition.ConditionType](entity, condition, policyStmt)
			if match {
				db.Logger.Println("Condition match found")
				anyConditionsMatch = true
				addConditiontoList = true
			} else {
				allConditionsMatch = false
			}
		}
		if addConditiontoList == true {
			conditionsList = append(conditionsList, condition)
		}
	}
	if policyStmt.MatchConditions == "all" && allConditionsMatch == true {
		return true, conditionsList
	}
	if policyStmt.MatchConditions == "any" && anyConditionsMatch == true {
		return true, conditionsList
	}
	return match, conditionsList
}
func (db *PolicyEngineDB) PolicyEngineApplyPolicyStmt(entity *PolicyEngineFilterEntityParams, policy Policy,
	policyStmt PolicyStmt, policyPath int, params interface{}, hit *bool, deleted *bool) {
	db.Logger.Println("policyEngineApplyPolicyStmt - ", policyStmt.Name)
	var conditionList []PolicyCondition
	conditionInfoList := make([]interface{}, 0)
	var match bool
	if policyStmt.Conditions == nil {
		db.Logger.Println("No policy conditions")
		*hit = true
	} else {
		//match, ret_conditionList := db.PolicyEngineMatchConditions(*entity, policyStmt)
		match, conditionList = db.PolicyEngineMatchConditions(*entity, policyStmt)
		db.Logger.Println("match = ", match)
		*hit = match
		if !match {
			db.Logger.Println("Conditions do not match")
			return
		}
		for j := 0; j < len(conditionList); j++ {
			conditionInfoList = append(conditionInfoList, conditionList[j].ConditionInfo)
		}
	}
	actionList := db.PolicyEngineImplementActions(*entity, policyStmt, conditionInfoList, params)
	if db.ActionListHasAction(actionList, policyCommonDefs.PolicyActionTypeRouteDisposition, "Reject") {
		db.Logger.Println("Reject action was applied for this entity")
		*deleted = true
	}
	//check if the route still exists - it may have been deleted by the previous statement action
	if db.IsEntityPresentFunc != nil {
		*deleted = !(db.IsEntityPresentFunc(params))
	}
	if db.UpdateEntityDB != nil {
		policyDetails := PolicyDetails{Policy: policy.Name, PolicyStmt: policyStmt.Name, ConditionList: conditionList, ActionList: actionList, EntityDeleted: *deleted}
		db.UpdateEntityDB(policyDetails, params)
	}
	db.AddPolicyEntityMapEntry(*entity, policy.Name, policyStmt.Name, conditionList, actionList)
}

func (db *PolicyEngineDB) PolicyEngineApplyPolicy(entity *PolicyEngineFilterEntityParams, policy Policy, policyPath int, params interface{}, hit *bool) {
	db.Logger.Println("policyEngineApplyPolicy - ", policy.Name)
	var policyStmtKeys []int
	deleted := false
	for k := range policy.PolicyStmtPrecedenceMap {
		db.Logger.Println("key k = ", k)
		policyStmtKeys = append(policyStmtKeys, k)
	}
	sort.Ints(policyStmtKeys)
	for i := 0; i < len(policyStmtKeys); i++ {
		db.Logger.Println("Key: ", policyStmtKeys[i], " policyStmtName ", policy.PolicyStmtPrecedenceMap[policyStmtKeys[i]])
		policyStmt := db.PolicyStmtDB.Get((patriciaDB.Prefix(policy.PolicyStmtPrecedenceMap[policyStmtKeys[i]])))
		if policyStmt == nil {
			db.Logger.Println("Invalid policyStmt")
			continue
		}
		db.PolicyEngineApplyPolicyStmt(entity, policy, policyStmt.(PolicyStmt), policyPath, params, hit, &deleted)
		if deleted == true {
			db.Logger.Println("Entity was deleted as a part of the policyStmt ", policy.PolicyStmtPrecedenceMap[policyStmtKeys[i]])
			break
		}
		if *hit == true {
			if policy.MatchType == "any" {
				db.Logger.Println("Match type for policy ", policy.Name, " is any and the policy stmt ", (policyStmt.(PolicyStmt)).Name, " is a hit, no more policy statements will be executed")
				break
			}
		}
	}
}
func (db *PolicyEngineDB) PolicyEngineApplyForEntity(entity PolicyEngineFilterEntityParams, policyData interface{}, params interface{}) {
	db.Logger.Println("policyEngineApplyForEntity")
	policy := policyData.(Policy)
	policyHit := false
	if len(entity.PolicyList) == 0 {
		db.Logger.Println("This route has no policy applied to it so far, just apply the new policy")
		db.PolicyEngineApplyPolicy(&entity, policy, policyCommonDefs.PolicyPath_All, params, &policyHit)
	} else {
		db.Logger.Println("This route already has policy applied to it - len(route.PolicyList) - ", len(entity.PolicyList))

		for i := 0; i < len(entity.PolicyList); i++ {
			db.Logger.Println("policy at index ", i)
			policyInfo := db.PolicyDB.Get(patriciaDB.Prefix(entity.PolicyList[i]))
			if policyInfo == nil {
				db.Logger.Println("Unexpected: Invalid policy in the route policy list")
			} else {
				oldPolicy := policyInfo.(Policy)
				if !isPolicyTypeSame(oldPolicy, policy) {
					db.Logger.Println("The policy type applied currently is not the same as new policy, so apply new policy")
					db.PolicyEngineApplyPolicy(&entity, policy, policyCommonDefs.PolicyPath_All, params, &policyHit)
				} else if oldPolicy.Precedence < policy.Precedence {
					db.Logger.Println("The policy types are same and precedence of the policy applied currently is lower than the new policy, so do nothing")
					return
				} else {
					db.Logger.Println("The new policy's precedence is lower, so undo old policy's actions and apply the new policy")
					db.PolicyEngineUndoPolicyForEntity(entity, oldPolicy, params)
					db.PolicyEngineApplyPolicy(&entity, policy, policyCommonDefs.PolicyPath_All, params, &policyHit)
				}
			}
		}
	}
}
func (db *PolicyEngineDB) PolicyEngineReverseGlobalPolicyStmt(policy Policy, policyStmt PolicyStmt) {
	db.Logger.Println("policyEngineApplyGlobalPolicyStmt - ", policyStmt.Name)
	var conditionItem interface{} = nil
	//global policies can only have statements with 1 condition and 1 action
	if policyStmt.Actions == nil {
		db.Logger.Println("No policy actions defined")
		return
	}
	if policyStmt.Conditions == nil {
		db.Logger.Println("No policy conditions")
	} else {
		if len(policyStmt.Conditions) > 1 {
			db.Logger.Println("only 1 condition allowed for global policy stmt")
			return
		}
		conditionItem = db.PolicyConditionsDB.Get(patriciaDB.Prefix(policyStmt.Conditions[0]))
		if conditionItem == nil {
			db.Logger.Println("Condition ", policyStmt.Conditions[0], " not found")
			return
		}
		actionItem := db.PolicyActionsDB.Get(patriciaDB.Prefix(policyStmt.Actions[0]))
		if actionItem == nil {
			db.Logger.Println("Action ", policyStmt.Actions[0], " not found")
			return
		}
		actionInfo := actionItem.(PolicyAction)
		if db.UndoActionfuncMap[actionInfo.ActionType] != nil {
			//since global policies have just 1 condition, we can pass that as the params to the undo call
			db.UndoActionfuncMap[actionInfo.ActionType](actionItem, nil, conditionItem, policyStmt)
		}
	}
}
func (db *PolicyEngineDB) PolicyEngineApplyGlobalPolicyStmt(policy Policy, policyStmt PolicyStmt) {
	db.Logger.Println("policyEngineApplyGlobalPolicyStmt - ", policyStmt.Name)
	var conditionItem interface{} = nil
	//global policies can only have statements with 1 condition and 1 action
	if policyStmt.Actions == nil {
		db.Logger.Println("No policy actions defined")
		return
	}
	if policyStmt.Conditions == nil {
		db.Logger.Println("No policy conditions")
	} else {
		if len(policyStmt.Conditions) > 1 {
			db.Logger.Println("only 1 condition allowed for global policy stmt")
			return
		}
		conditionItem = db.PolicyConditionsDB.Get(patriciaDB.Prefix(policyStmt.Conditions[0]))
		if conditionItem == nil {
			db.Logger.Println("Condition ", policyStmt.Conditions[0], " not found")
			return
		}
		policyCondition := conditionItem.(PolicyCondition)
		conditionInfoList := make([]interface{}, 0)
		conditionInfoList = append(conditionInfoList, policyCondition.ConditionInfo)

		actionItem := db.PolicyActionsDB.Get(patriciaDB.Prefix(policyStmt.Actions[0]))
		if actionItem == nil {
			db.Logger.Println("Action ", policyStmt.Actions[0], " not found")
			return
		}
		actionInfo := actionItem.(PolicyAction)
		if db.ActionfuncMap[actionInfo.ActionType] != nil {
			db.ActionfuncMap[actionInfo.ActionType](actionInfo.ActionInfo, conditionInfoList, nil)
		}
	}
}
func (db *PolicyEngineDB) PolicyEngineReverseGlobalPolicy(policy Policy) {
	db.Logger.Println("policyEngineReverseGlobalPolicy")
	var policyStmtKeys []int
	for k := range policy.PolicyStmtPrecedenceMap {
		db.Logger.Println("key k = ", k)
		policyStmtKeys = append(policyStmtKeys, k)
	}
	sort.Ints(policyStmtKeys)
	for i := 0; i < len(policyStmtKeys); i++ {
		db.Logger.Println("Key: ", policyStmtKeys[i], " policyStmtName ", policy.PolicyStmtPrecedenceMap[policyStmtKeys[i]])
		policyStmt := db.PolicyStmtDB.Get((patriciaDB.Prefix(policy.PolicyStmtPrecedenceMap[policyStmtKeys[i]])))
		if policyStmt == nil {
			db.Logger.Println("Invalid policyStmt")
			continue
		}
		db.PolicyEngineReverseGlobalPolicyStmt(policy, policyStmt.(PolicyStmt))
	}
}
func (db *PolicyEngineDB) PolicyEngineApplyGlobalPolicy(policy Policy) {
	db.Logger.Println("policyEngineApplyGlobalPolicy")
	var policyStmtKeys []int
	for k := range policy.PolicyStmtPrecedenceMap {
		db.Logger.Println("key k = ", k)
		policyStmtKeys = append(policyStmtKeys, k)
	}
	sort.Ints(policyStmtKeys)
	for i := 0; i < len(policyStmtKeys); i++ {
		db.Logger.Println("Key: ", policyStmtKeys[i], " policyStmtName ", policy.PolicyStmtPrecedenceMap[policyStmtKeys[i]])
		policyStmt := db.PolicyStmtDB.Get((patriciaDB.Prefix(policy.PolicyStmtPrecedenceMap[policyStmtKeys[i]])))
		if policyStmt == nil {
			db.Logger.Println("Invalid policyStmt")
			continue
		}
		db.PolicyEngineApplyGlobalPolicyStmt(policy, policyStmt.(PolicyStmt))
	}
}

func (db *PolicyEngineDB) PolicyEngineTraverseAndApplyPolicy(policy Policy) {
	db.Logger.Println("PolicyEngineTraverseAndApplyPolicy -  apply policy ", policy.Name)
	if policy.ExportPolicy || policy.ImportPolicy {
		db.Logger.Println("Applying import/export policy to all routes")
		if db.TraverseAndApplyPolicyFunc != nil {
			db.Logger.Println("Calling TraverseAndApplyPolicyFunc function")
			db.TraverseAndApplyPolicyFunc(policy, db.PolicyEngineApplyForEntity)
		}
	} else if policy.GlobalPolicy {
		db.Logger.Println("Need to apply global policy")
		db.PolicyEngineApplyGlobalPolicy(policy)
	}
}

func (db *PolicyEngineDB) PolicyEngineTraverseAndReversePolicy(policy Policy) {
	db.Logger.Println("PolicyEngineTraverseAndReversePolicy -  reverse policy ", policy.Name)
	if policy.ExportPolicy || policy.ImportPolicy {
		db.Logger.Println("Reversing import/export policy ")
		db.TraverseAndReversePolicyFunc(policy)
	} else if policy.GlobalPolicy {
		db.Logger.Println("Need to reverse global policy")
		db.PolicyEngineReverseGlobalPolicy(policy)
	}
}

func (db *PolicyEngineDB) PolicyEngineFilter(entity PolicyEngineFilterEntityParams, policyPath int, params interface{}) {
	db.Logger.Println("PolicyEngineFilter")
	var policyPath_Str string
	if policyPath == policyCommonDefs.PolicyPath_Import {
		policyPath_Str = "Import"
	} else if policyPath == policyCommonDefs.PolicyPath_Export {
		policyPath_Str = "Export"
	} else if policyPath == policyCommonDefs.PolicyPath_All {
		policyPath_Str = "ALL"
		db.Logger.Println("policy path ", policyPath_Str, " unexpected in this function")
		return
	}
	db.Logger.Println("PolicyEngineFilter for policypath ", policyPath_Str, "create = ", entity.CreatePath, " delete = ", entity.DeletePath, " route: ", entity.DestNetIp, " protocol type: ", entity.RouteProtocol)
	var policyKeys []int
	var policyHit bool
	idx := 0
	var policyInfo interface{}
	if policyPath == policyCommonDefs.PolicyPath_Import {
		for k := range db.ImportPolicyPrecedenceMap {
			policyKeys = append(policyKeys, k)
		}
	} else if policyPath == policyCommonDefs.PolicyPath_Export {
		for k := range db.ExportPolicyPrecedenceMap {
			policyKeys = append(policyKeys, k)
		}
	}
	sort.Ints(policyKeys)
	for {
		if entity.DeletePath == true { //policyEngineFilter called during delete
			if entity.PolicyList != nil {
				if idx >= len(entity.PolicyList) {
					break
				}
				db.Logger.Println("getting policy ", idx, " from entity.PolicyList")
				policyInfo = db.PolicyDB.Get(patriciaDB.Prefix(entity.PolicyList[idx]))
				idx++
				if policyInfo.(Policy).ExportPolicy && policyPath == policyCommonDefs.PolicyPath_Import || policyInfo.(Policy).ImportPolicy && policyPath == policyCommonDefs.PolicyPath_Export {
					db.Logger.Println("policy ", policyInfo.(Policy).Name, " not the same type as the policypath -", policyPath_Str)
					continue
				}
			} else {
				db.Logger.Println("PolicyList empty and this is a delete operation, so break")
				break
			}
		} else if entity.CreatePath == true { //policyEngine filter called during create
			db.Logger.Println("idx = ", idx, " len(policyKeys):", len(policyKeys))
			if idx >= len(policyKeys) {
				break
			}
			policyName := ""
			if policyPath == policyCommonDefs.PolicyPath_Import {
				policyName = db.ImportPolicyPrecedenceMap[policyKeys[idx]]
			} else if policyPath == policyCommonDefs.PolicyPath_Export {
				policyName = db.ExportPolicyPrecedenceMap[policyKeys[idx]]
			}
			db.Logger.Println("getting policy  ", idx, " policyKeys[idx] = ", policyKeys[idx], " ", policyName, " from PolicyDB")
			policyInfo = db.PolicyDB.Get((patriciaDB.Prefix(policyName)))
			idx++
		}
		if policyInfo == nil {
			db.Logger.Println("Nil policy")
			continue
		}
		policy := policyInfo.(Policy)
		localPolicyDB := *db.LocalPolicyDB
		if localPolicyDB != nil && localPolicyDB[policy.LocalDBSliceIdx].IsValid == false {
			db.Logger.Println("Invalid policy at localDB slice idx ", policy.LocalDBSliceIdx)
			continue
		}
		db.PolicyEngineApplyPolicy(&entity, policy, policyPath, params, &policyHit)
		if policyHit {
			db.Logger.Println("Policy ", policy.Name, " applied to the route")
			break
		}
	}
	if entity.PolicyHitCounter == 0 {
		db.Logger.Println("Need to apply default policy, policyPath = ", policyPath, "policyPath_Str= ", policyPath_Str)
		if policyPath == policyCommonDefs.PolicyPath_Import {
			db.Logger.Println("Applying default import policy")
			if db.DefaultImportPolicyActionFunc != nil {
				db.DefaultImportPolicyActionFunc(nil, nil, params)
			}
		} else if policyPath == policyCommonDefs.PolicyPath_Export {
			db.Logger.Println("Applying default export policy")
			if db.DefaultExportPolicyActionFunc != nil {
				db.DefaultExportPolicyActionFunc(nil, nil, params)
			}
		}
	}
	if entity.DeletePath == true {
		db.DeletePolicyEntityMapEntry(entity, "")
	}
}