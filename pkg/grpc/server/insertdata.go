package server

import (
	"encoding/json"
	"fiy/app/cmdb/models/model"
	"fiy/app/cmdb/models/resource"
	orm "fiy/common/global"
	"fiy/common/log"

	"gorm.io/gorm/clause"
)

/*
  @Author : lanyulei
  @Desc : 存入数据
*/

func formatData(data string) (result map[string]interface{}, err error) {
	var (
		modelIDs []struct {
			ID         int    `json:"id"`
			Identifies string `json:"identifies"`
		}
		jsonData []byte
	)

	result = make(map[string]interface{})

	// 查询模型ID
	err = orm.Eloquent.Model(&model.Info{}).Where("identifies in ?", []string{
		"built_in_idc_host",
		"built_in_gpu",
		"built_in_memory",
		"built_in_cpu",
		"built_in_disk",
		"built_in_net",
	}).Find(&modelIDs).Error
	if err != nil {
		log.Info("查询模型ID失败，", err)
		return
	}

	modelIDMaps := make(map[string]int)
	for _, i := range modelIDs {
		modelIDMaps[i.Identifies] = i.ID
	}

	dataMap := map[string]interface{}{}
	err = json.Unmarshal([]byte(data), &dataMap)
	if err != nil {
		log.Info("json反序列化失败，", err)
		return
	}

	for k, v := range dataMap {
		if k == "info" {
			jsonData, err = json.Marshal(v.(map[string]interface{})["data"])
			result[k] = &resource.Data{
				Uuid:   v.(map[string]interface{})["uuid"].(string),
				InfoId: modelIDMaps[v.(map[string]interface{})["info_uuid"].(string)],
				Status: int(v.(map[string]interface{})["status"].(float64)),
				Data:   jsonData,
			}
		} else {
			dataList := make([]resource.Data, 0)
			for _, d := range v.([]interface{}) {
				jsonData, err = json.Marshal(d.(map[string]interface{})["data"])
				dataList = append(dataList, resource.Data{
					Uuid:   d.(map[string]interface{})["uuid"].(string),
					InfoId: modelIDMaps[d.(map[string]interface{})["info_uuid"].(string)],
					Status: int(d.(map[string]interface{})["status"].(float64)),
					Data:   jsonData,
				})
			}
			result[k] = &dataList
		}
	}

	return
}

func insertData(data string) (err error) {
	var (
		result          map[string]interface{}
		hostInfo        resource.Data
		dataList        []resource.Data
		dataRelatedList []resource.DataRelated
	)

	result, err = formatData(data)
	if err != nil {
		log.Error("重组数据失败，", err)
		return
	}

	tx := orm.Eloquent.Begin()

	// 查询ID
	err = orm.Eloquent.Model(&resource.Data{}).
		Where("uuid = ?", result["info"].(*resource.Data).Uuid).
		Find(&hostInfo).Error
	if err != nil {
		log.Error("查询数据ID失败，", err)
		tx.Rollback()
		return
	}

	for k, d := range result {
		err = tx.Model(&resource.Data{}).
			Clauses(clause.OnConflict{
				Columns:   []clause.Column{{Name: "uuid"}},
				DoUpdates: clause.AssignmentColumns([]string{"info_id", "status", "data"}),
			}).Create(d).Error
		if err != nil {
			tx.Rollback()
			log.Error("同步数据失败，", err)
			return
		}

		if k != "info" {
			dataUuids := make([]string, 0)
			for _, z := range *d.(*[]resource.Data) {
				dataUuids = append(dataUuids, z.Uuid)
			}
			err = orm.Eloquent.Where("uuid in ?", dataUuids).Find(&dataList).Error
			if err != nil {
				log.Error("查询数据列表失败，", err)
				tx.Rollback()
				return
			}

			for _, z := range dataList {
				dataRelatedList = append(dataRelatedList, resource.DataRelated{
					Source:       hostInfo.Id,
					Target:       z.Id,
					SourceInfoId: hostInfo.InfoId,
					TargetInfoId: z.InfoId,
				})
			}
		}
	}

	err = tx.Create(&dataRelatedList).Error
	if err != nil {
		log.Error("创建数据关联失败")
		tx.Rollback()
		return
	}

	tx.Commit()

	return
}
