-- REF: https://gcis.nat.gov.tw/mainNew/matterAction.do?method=showFile&fileNo=t70215_p

INSERT INTO ref_ledger_first_grade (
    first_grade,
    type_name,
    type_name_zh,
    description_en,
    description_zh
) VALUES
('1', 'Assets', '資產',
'Economic resources controlled by an entity as a result of past transactions or events and from which future economic benefits probably are obtained.',
'指商業透過交易或其他事項所獲得之經濟資源，能以貨幣衡量並預期未來能提供經濟效益者。'),
('2', 'Liabilities', '負債',
'An obligation of an entity arising from past transactions or events, the settlement of which may result in the transfer or use of assets, provision of services or other yielding of economic benefits in the future.',
'指商業由於過去之交易或其他事項，所產生之經濟義務，能以貨幣衡量，並將以提供勞務或支付經濟資源之方式償付者。'),
('3', 'Owners equity', '業主權益',
'Owners equity equals to that of total assets minus total liability.',
'指商業之全部資產減除全部負債後之餘額，歸屬業主之權益。'),
('4', 'Operating revenue', '營業收入',
'Revenue from operating activities during this period.',
'指本期內因經常營業活動而銷售商品或提供勞務等所獲得之收入。'),
('5', 'Operating costs', '營業成本',
'Cost from operating activities during this period.',
'指本期內因銷售商品或提供勞務等而應負擔之成本。'),
('6', 'Operating expenses', '營業費用',
'Expenses arising from selling products or services.',
'指本期內銷售商品或提供勞務等所應負擔之費用。'),
('7', 'Non-operating revenue and expenses', '營業外收益及費損',
'The revenue and expenses not arising from operating activities.',
'指本期內非因經常營業活動所發生之收益及費損。'),
('A', 'Personal or household revenue', '個人或家庭收益',
'The personal or household income within the current period is defined by the system itself and is distinguished from the general accounting subjects.',
'指本期內個人或家庭運作收入，由本系統自定義，以和一般會計科目做區隔。'),
('B', 'Personal or household expenses', '個人或家庭費損',
'The personal or household expenses within the current period is defined by the system itself and is distinguished from the general accounting subjects.',
'指本期內個人或家庭運作費損，由本系統自定義，以和一般會計科目做區隔。');

INSERT INTO ref_ledger_second_grade (
    first_grade,
    second_grade,
    type_name,
    type_name_zh,
    description_en,
    description_zh
) VALUES
('1', '11', 'Current assets', '流動資產', 'Current assets are cash and other assets expected to be converted to cash, sold, or consumed within a year', '指現金、短期投資及其他預期能於一年內變現或耗用之資產。'),
('1', '12', 'Current assets', '流動資產', 'Current assets are cash and other assets expected to be converted to cash, sold, or consumed within a year', '指現金、短期投資及其他預期能於一年內變現或耗用之資產。'),
('1', '13', 'Funds and long-term investments', '基金及長期投資', 'Assets and long-term investments that are designated for specific purposes.', '指商業為特定用途而提撥之各類基金及因業務目的而為長期性之投資。'),
('1', '14', 'Fixed assets', '固定資產', 'Assets which are purchased for continued and long-term use in earning profit in a business. They are written off against profits over their anticipated life by charging depreciation expenses (with exception of land). Accumulated depreciation is shown in the face of the balance sheet or in the notes.', '指為供營業上使用，非以出售為目的，且使用年限在一年以上之有形資產，除土地外，應於達到可供使用狀態時，以合理而有系統之方法，按期提列折舊，其累計折舊應列為固定資產之減項。'),
('1', '15', 'Fixed assets', '固定資產', 'Assets which are purchased for continued and long-term use in earning profit in a business. They are written off against profits over their anticipated life by charging depreciation expenses (with exception of land). Accumulated depreciation is shown in the face of the balance sheet or in the notes.', '指為供營業上使用，非以出售為目的，且使用年限在一年以上之有形資產，除土地外，應於達到可供使用狀態時，以合理而有系統之方法，按期提列折舊，其累計折舊應列為固定資產之減項。'),
('1', '16', 'Depletable assets', '遞耗資產', 'Natural resources, the value of which will be exhausted by mining, cutting and other consumption methods.', '指資產價值將隨開採、砍伐或其他使用方法而耗竭之天然資源。'),
('1', '17', 'Intangible assets', '無形資產', 'The assets lacked physical substance but qualified economic value.', '指無實體存在而具經濟價值之資產。'),
('1', '18', 'Other assets', '其他資產 ', 'Assets that cannot be classified into the asset headings above and which has recoverable period longer than one year.', '指不能歸屬於前五項之資產，且回收或變現期限在一年以上者，以較長者為準。'),

('2', '21', 'Current liabilities', '流動負債', 'Liabilities which are reasonably expected to be liquidated with current assets or other current liabilities within a year.', '指將於一年內，以流動資產或其他流動負債償付之債務。'),
('2', '22', 'Current liabilities', '流動負債', 'Liabilities which are reasonably expected to be liquidated with current assets or other current liabilities within a year.', '指將於一年內，以流動資產或其他流動負債償付之債務。'),
('2', '23', 'Long-term liabilities', '長期負債', 'The liabilities are reasonably expected not to be liquidated within a year.', '指到期日在一年以上之債務。'),
('2', '28', 'Other liabilities', '其他負債', 'Liabilities that cannot be classified into the liabilities headings above', '凡不屬於上列各項負債皆屬之。'),

('3', '31', 'Capital', '資本', 'Capital contributed by business owners and registered with the competent authority in charge but not including preferred stock liability.', '業主對商業投入之資本，並向主管機關登記者，但不包括符合負債性質之特別股。'),
('3', '32', 'Cdditional paid-in capital', '資本公積', 'The equity not arising from the operating results.', '指非由營業結果所產生之權益。'),
('3', '33', 'Retained earnings (accumulated deficit)', '保留盈餘(或累積虧損)', 'Net income that have been retained by the corporation at year-end. If the opposite occurs when the corporation has net losses the corporation retains those losses at year-end.', '指由營業結果所產生之權益。'),
('3', '34', 'Equity adjustments', '權益調整', 'Other items increasing or decreasing the owners equity.', '指其他造成業主權益增加或減少之項目。'),
('3', '36', 'Minority interest', '少數股權', 'A subsidiarys equity that is held by the investors other than these affiliated companies', '指聯屬公司以外之投資者持有子公司之股份權益。'),

('4', '41', 'Sales revenue', '銷貨收入', 'Income earned from selling goods.', '指因銷售商品所賺得之收入。'),
('4', '46', 'Service revenue', '勞務收入', 'Revenues earned from providing services.', '指因提供勞務所賺得之收入。'),
('4', '47', 'Agency revenue', '業務收入', 'Revenues earned from compensation for intermediary and agent business or for acting as an assignee.', '指因居間及代理業務或受委託等報酬所得之收入。'),
('4', '48', 'Other operating revenue', '其他營業收入－其他', 'Other operating revenues that cannot be classified into the headings above.', '指不能歸屬於前述各款之其他營業收入。'),

('5', '51', 'Cost of goods sold', '銷貨成本', 'Refer to the original costs of merchandise sold or the production costs of goods sold.', '指銷售商品之原始成本或產品之製造成本。'),
('5', '56', 'Service costs', '勞務成本', 'Costs incurred for providing services.', '指提供勞務所應負擔之成本。'),
('5', '57', 'Agency costs', '業務成本', 'Costs incurred for intermediary and agent business or for acting as an assignee.', '指因居間及代理業務或受委託等所應負擔之成本。'),
('5', '58', 'Other operating costs', '其他營業成本', 'Expense incurred for earning other operating revenues.', '指因其他營業收入所應負擔之成本。'),

('6', '61', 'Operating expenses', '營業費用', 'Expenses arising from selling products or services.', '指本期內銷售商品或提供勞務等所應負擔之費用。'),
('6', '62', 'General & administrative expenses', '管理及總務費用', 'Any expense incurred in the administrative and general departments.', '凡管理及總務部門發生之費用。'),
('6', '63', 'Research and development expenses', '研究及發展費用', 'Research, improvement and experiment expenses incurred for research and developing new products, improving production technology, technology for providing services and production process.', '凡為研究發展新產品、改進生產技術、改進提供勞務技術及改善製程而發生之各項研究、改良、實驗等費用皆屬之。'),

('7', '71', 'Non-operating revenue', '營業外收益', 'The revenue not arising from operating activities.', '指本期內非因經常營業活動所發生之收益。'),
('7', '72', 'Non-operating revenue', '營業外收益', 'The revenue not arising from operating activities.', '指本期內非因經常營業活動所發生之收益。'),
('7', '73', 'Non-operating revenue', '營業外收益', 'The revenue not arising from operating activities.', '指本期內非因經常營業活動所發生之收益。'),
('7', '74', 'Non-operating revenue', '營業外收益', 'The revenue not arising from operating activities.', '指本期內非因經常營業活動所發生之收益。'),
('7', '75', 'Non-operating expenses', '營業外費損', 'The expenses not arising from operating activities.', '指本期內非因經常營業活動所發生之費損。'),
('7', '76', 'Non-operating expenses', '營業外費損', 'The expenses not arising from operating activities.', '指本期內非因經常營業活動所發生之費損。'),
('7', '77', 'Non-operating expenses', '營業外費損', 'The expenses not arising from operating activities.', '指本期內非因經常營業活動所發生之費損。'),
('7', '78', 'Non-operating expenses', '營業外費損', 'The expenses not arising from operating activities.', '指本期內非因經常營業活動所發生之費損。'),

('A', 'A1', 'Common revenue', '一般收益', 'General and direct household income includes the sale of goods or the provision of services.', '一般性及直接性的家庭收入, 包含貨品的銷售或服務的提供'),
('A', 'A2', 'Investment revenue', '投資收益', 'Investment-related household income includes various types of interest on long and short-term assets, disposal, valuation gains, etc.', '投資相關的家庭收入, 包含各種長短期的資產利息, 處分, 評價收益等'),
('A', 'A8', 'Other revenue', '其他收益', 'Other household revenues that cannot be classified into the headings above.', '凡不屬於上列家庭收益者皆屬之'),

('B', 'B1', 'Common expenses', '一般支出', 'General household expenses, including necessary and discretionary expenses.', '一般性的家庭支出, 包含必要及非必要性費用等'),
('B', 'B2', 'Investment loss and liability expenses', '投資損失和債務支出', 'Investment or debt-related household expenses include interest, disposal, or evaluation losses.', '投資或債務相關家庭費用支出, 包含利息, 處置或評價損失等'),
('B', 'B8', 'Other expenses', '其他支出', 'Other household expenses that cannot be classified into the headings above.', '凡不屬於上列家庭費損者皆屬之');

INSERT INTO ref_ledger_third_grade (
    first_grade,
    second_grade,
    third_grade,
    type_name,
    type_name_zh,
    description_en,
    description_zh
) VALUES
('1', '11', '111', 'Cash and cash equivalents', '現金及約當現金', 'Cash on hand, demand deposits, and short-term, highly liquid certificate deposits or investments that are readily convertible to known amounts of cash and which are subject to an insignificant risk of changes in value.', '指庫存現金、活期存款及可隨時轉換成定額現金且價值變動風險甚小之短期並具高度流動性之定期存款或投資。'),
('1', '11', '112', 'Short-term investments', '短期投資', 'Consists of financial assets at fair value through income statement, financial assets in available for sale and financial assets in held to maturity-current.', '指短期性之投資，包括公平價值變動列入損益之金融資產、備供出售金融資產及持有到期日金融資產。'),
('1', '11', '113', 'Notes receivable', '應收票據', 'A written promise that is expected to be collected by a business and should take stock the amounts uncollectible as settlement. Then account reasonable allowance to be the deduction of note receivable.', '商業應收之各種票據。結算時應評估應收票據無法收現之金額，提列適當之備抵呆帳，列為應收票據之減項。'),
('1', '12', '121', 'Inventories', '存貨', 'Products, finished goods, by-products that are available for sale under normal operation; or work-in process being processed that is expected to be sold when completed; or materials or supplies that are expected to be used directly or indirectly in producing the goods (or services) available for sale.', '指備供正常營業出售之商品、製成品、副產品；或正在生產中之在製品，將於加工完成後出售者；或將直接、間接用於生產供出售之商品（或勞務）之材料或物料。'),
('1', '12', '122', 'Inventories', '存貨', 'Products, finished goods, by-products that are available for sale under normal operation; or work-in process being processed that is expected to be sold when completed; or materials or supplies that are expected to be used directly or indirectly in producing the goods (or services) available for sale.', '指備供正常營業出售之商品、製成品、副產品；或正在生產中之在製品，將於加工完成後出售者；或將直接、間接用於生產供出售之商品（或勞務）之材料或物料。'),
('1', '13', '131', 'Funds', '基金', 'Assets that are designated for specific purposes, for example, redemption fund, fund for improvement and expansion and contingency fund.', '指為特定用途所提列之資產，如償債基金、改良及擴充基金、意外損失準備基金等。'),
('1', '13', '132', 'Long-term investments', '長期投資', 'Long-term investments including investing in stock bonds and real estate.', '指長期性之投資，如投資其他企業發行之股票、債券或投資不動產等。'),
('1', '13', '134', 'Long-term investments', '長期投資', 'Long-term investments including investing in stock bonds and real estate.', '指長期性之投資，如投資其他企業發行之股票、債券或投資不動產等。'),
('1', '14', '141', 'Land', '土地', 'Land and perpetual land improvements for operating use.', '指營業上使用之土地及具有永久性之土地改良。'),
('1', '14', '143', 'Buildings', '房屋及建築', 'Self-owned buildings and their auxiliary equipment available for use in operation.', '指營業上使用之自有房屋建築及其他附屬設備。'),
('1', '14', '144', 'Machinery and equipment', '機(器)具及設備', 'Self-owned machinery that are used directly or indirectly in production, transportation equipment, office equipment and other equipment.', '指自有之直接或間接提供生產之機（器）具、運輸設備、辦公設備及其各項設備零配件。'),
('1', '15', '151', 'Leased assets', '租賃資產', 'Assets leased under capital lease contracts.', '指依資本租賃契約所承租之資產。'),
('1', '15', '156', 'Construction in progress and prepayments for equipment', '未完工程及預付購置設備款', 'Construction under progress or working in process, and prepayments for equipment purchased for use in operation.', '指正在建造或裝置而尚未完竣之工程及預付購置供營業使用之設備款項等。'),
('1', '15', '158', 'Miscellaneous property, plant, and equipment', '雜項固定資產', 'Assets that cannot be classified under the asset headings above.', '指不能歸屬於前述各款之資產。'),
('1', '17', '171', 'Trademarks', '商標權', 'Trademark right held or purchased legally; valued at unamortized costs', '指依法取得或購入之商標權。'),
('1', '17', '172', 'Patents', '專利權', 'The patent right held or purchased legally.', '指依法取得或購入之專利權。'),
('1', '17', '174', 'Copyright', '著作權', 'Copyright held or purchased legally for the publishing and sale of original composition or translation of literature, art, academic article, music, motion picture and other similar works, and the right of performing art and music; valued at its unamortized costs.', '指依法取得或購入文學、藝術、學術、音樂、電影等創作或翻譯之出版、銷售、表演等權利。'),
('1', '17', '175', 'Computer software', '電腦軟體', 'Computer software purchased or developed for sale, rent or other form of marketing.', '指對於購買或開發以供出售、出租或以其他方式行銷之電腦軟體。'),
('1', '17', '176', 'Goodwill', '商譽', 'Goodwill acquired as a result of a purchase.', '指出價取得之商譽。'),
('1', '18', '181', 'Deferred assets', '遞延資產', 'Expenditures incurred that will benefit over one year or one operating cycle and should be amortized over the future periods.', '指已發生之支出，其效益超過一年，應由以後各期負擔者。'),
('1', '18', '182', 'Idle assets', '閒置資產', 'Assets not under operating use currently.', '指目前未供營業上使用之資產。'),
('1', '18', '185', 'Assets leased to others', '出租資產', 'Self-owned assets held for rent by a business which is not in the investment or leasing business', '指非以投資或出租為業之商業供作出租之自有資產。'),
('1', '18', '188', 'Miscellaneous assets', '雜項資產', 'Other assets that cannot be classified under the other asset headings above.', '指不能歸屬於前述各款之其他資產。'),

('2', '21', '211', 'Short-term debt', '短期借款', 'Loan or overdraft borrowed from financial institutions or other personal creditors, due within one year or one operating cycle.', '指向金融機構或他人借入及透支之款項，其償還期限在一年以內者。'),
('2', '21', '213', 'Notes payable', '應付票據', 'Various notes to be paid by the business.', '指商業應付之各種票據。'),
('2', '21', '217', 'Accrued expenses', '應付費用', 'Expense incurred but not yet paid, including accrued payroll, accrued rent payable, accrued interest payable, accrued VAT payable, accrued taxes payable-other and other accrued expense payable.', '凡已發生而尚未支付之各項應付費用，包括應付薪工﹑租金﹑利息﹑營業稅﹑應付其他稅捐及其他應付費用等皆屬之。'),
('2', '21', '219', 'Other payables', '其他應付款', 'Payables that cannot be classified asaccounts payable.', '指不能歸屬於應付帳款之款項。'),
('2', '22', '226', 'Unearned receipts', '預收款項', 'Various amounts collected in advance.', '指預為收納之各種款項。'),
('2', '23', '232', 'Long-term debt payable', '長期借款', 'Borrowing with due date beyond one year or one operating cycle.', '指到期日在一年以上之借款。'),
('2', '23', '233', 'Long-term notes payable', '長期應付票據及款項', 'Notes and accounts payable with repayment period beyond one year or one operating cycle.', '指付款期間在一年以上之應付票據、應付帳款等。'),
('2', '28', '288', 'Other liabilities', '其他負債', 'Liabilities that cannot be classified into the liabilities headings above', '凡不屬於上列各項負債皆屬之。'),

('3', '31', '311', 'Capital', '資本(或股本)', 'Capital contributed by business owners and registered with the competent authority in charge but not including preferred stock liability.', '業主對商業投入之資本，並向主管機關登記者，但不包括符合負債性質之特別股。'),
('3', '33', '335', 'Retained earnings unappropriated (or accumulated deficit)', '未分配盈餘(或累積虧損)', 'Earnings not yet appropriated or deficits accumulated not yet compensated.', '指未經指撥之盈餘或未經彌補之虧損。'),

('4', '41', '411', 'Sales revenue', '銷貨收入', 'Income earned from selling goods.', '指因銷售商品所賺得之收入。'),
('4', '46', '461', 'Service revenue', '勞務收入', 'Revenues earned from providing services.', '指因提供勞務所賺得之收入。'),
('4', '48', '488', 'Other operating revenue', '其他營業收入－其他', 'revenues that cannot be classified into the headings above.', '指不能歸屬於前述各款之其他營業收入。'),

('5', '51', '511', 'Cost of goods sold', '銷貨成本', 'Refer to the original costs of merchandise sold or the production costs of goods sold.', '指銷售商品之原始成本或產品之製造成本。'),
('5', '51', '512', 'Purchases', '進貨', 'Purchase of goods for sale. Purchase returns, discounts and allowances should be the deduction of purchases.', '凡購進待銷之貨品均屬之。進貨退回及折讓應作為進貨成本之減項。'),
('5', '51', '513', 'Material purchased', '進料', 'The acquisition costs of all materials that can be traced directly to the cost object, including freight-in, insurance, import duty, notary fee and rent', '進料（製造業適用）：本科目適用於對存貨處理採用定期盤存制之製造業。凡購進原料及物料所發生之進價及應負擔之運費﹑保險費﹑關稅﹑公證費﹑棧租等皆屬之。'),
('5', '51', '514', 'Direct labor', '直接人工', 'Labor cost that can be reasonably identified for the production of finished goods.', '指能合理辨認係直接歸屬於生產製成品所發生之人工成本。'),
('5', '51', '515', 'Manufacturing overhead', '製造費用', 'Costs that cannot be classified as material and direct labor in the manufacturing or the service department.', '適用於製造業，凡製造業製造部門因從事生產所發生之除原料及人工成本以外之費用，及服務部門所發生之費用皆屬之。'),
('5', '51', '516', 'Manufacturing overhead', '製造費用', 'Costs that cannot be classified as material and direct labor in the manufacturing or the service department.', '適用於製造業，凡製造業製造部門因從事生產所發生之除原料及人工成本以外之費用，及服務部門所發生之費用皆屬之。'),
('5', '51', '517', 'Manufacturing overhead', '製造費用', 'Costs that cannot be classified as material and direct labor in the manufacturing or the service department.', '適用於製造業，凡製造業製造部門因從事生產所發生之除原料及人工成本以外之費用，及服務部門所發生之費用皆屬之。'),
('5', '51', '518', 'Manufacturing overhead', '製造費用', 'Costs that cannot be classified as material and direct labor in the manufacturing or the service department.', '適用於製造業，凡製造業製造部門因從事生產所發生之除原料及人工成本以外之費用，及服務部門所發生之費用皆屬之。'),
('5', '56', '561', 'Service costs', '勞務成本', 'Costs incurred for providing services', ' 指提供勞務所應負擔之成本。'),
('5', '58', '588', 'Other operating costs', '其他營業成本—其他', 'Expense incurred for earning other operating revenues.', '指因其他營業收入所應負擔之成本。'),

('6', '61', '615', 'Selling expenses', '銷售費用', 'Expenses incurred in selling products.', '指本期內業務部門所發生之費用。'),
('6', '61', '616', 'Selling expenses', '銷售費用', 'Expenses incurred in selling products.', '指本期內業務部門所發生之費用。'),
('6', '61', '617', 'Selling expenses', '銷售費用', 'Expenses incurred in selling products.', '指本期內業務部門所發生之費用。'),
('6', '61', '618', 'Selling expenses', '銷售費用', 'Expenses incurred in selling products.', '指本期內業務部門所發生之費用。'),
('6', '62', '625', 'General & administrative expenses', '管理及總務費用', 'Any expense incurred in the administrative and general departments.', '凡管理及總務部門發生之費用。'),
('6', '62', '626', 'General & administrative expenses', '管理及總務費用', 'Any expense incurred in the administrative and general departments.', '凡管理及總務部門發生之費用。'),
('6', '62', '627', 'General & administrative expenses', '管理及總務費用', 'Any expense incurred in the administrative and general departments.', '凡管理及總務部門發生之費用。'),
('6', '62', '628', 'General & administrative expenses', '管理及總務費用', 'Any expense incurred in the administrative and general departments.', '凡管理及總務部門發生之費用。'),
('6', '63', '635', 'Research and development expenses', '研究及發展費用', 'Research, improvement and experiment expenses incurred for research and developing new products, improving production technology, technology for providing services and production process.', '凡為研究發展新產品、改進生產技術、改進提供勞務技術及改善製程而發生之各項研究、改良、實驗等費用皆屬之。'),
('6', '63', '636', 'Research and development expenses', '研究及發展費用', 'Research, improvement and experiment expenses incurred for research and developing new products, improving production technology, technology for providing services and production process.', '凡為研究發展新產品、改進生產技術、改進提供勞務技術及改善製程而發生之各項研究、改良、實驗等費用皆屬之。'),
('6', '63', '637', 'Research and development expenses', '研究及發展費用', 'Research, improvement and experiment expenses incurred for research and developing new products, improving production technology, technology for providing services and production process.', '凡為研究發展新產品、改進生產技術、改進提供勞務技術及改善製程而發生之各項研究、改良、實驗等費用皆屬之。'),
('6', '63', '638', 'Research and development expenses', '研究及發展費用', 'Research, improvement and experiment expenses incurred for research and developing new products, improving production technology, technology for providing services and production process.', '凡為研究發展新產品、改進生產技術、改進提供勞務技術及改善製程而發生之各項研究、改良、實驗等費用皆屬之。'),

('7', '71', '711', 'Interest revenue', '利息收入', 'Interest revenues resulting from deposits with financial institution or loan to others.', '指資金存放金融機構、借予他人或購買各種債務證券所產生之利息收入。'),
('7', '71', '714', 'Investment income', '投資收益', 'Investment income of a non-investment company, engaged in financing instruments.', '指非以投資為業之商業，投資金融商品所產生之收益。'),
('7', '71', '716', 'Gain on disposal of investments', '處分投資收益', 'Gain from disposal of short-term or long-term investments.', '凡因處分金融資產及長期投資所獲得之利益皆屬之。'),
('7', '71', '717', 'Gain on disposal of assets', '處分資產溢價收入', 'Gain from disposal of property, plant and equipment.', '凡因處分固定資產所獲得之利益屬之。'),
('7', '74', '748', 'Other non-operating revenue', '其他營業外收益', 'Other non-operating revenue that cannot be classified into the headings above.', '凡不屬於以上各項之營業外收益皆屬之。'),
('7', '75', '751', 'Interest expense', '利息費用', 'Interest expense incurred as a result of borrowing from financial institutions or other persons.', '凡向金融機構或他人借款等所發生之利息費用皆屬之。'),
('7', '75', '753', 'Investment loss', '投資損失', 'Investment loss of a non-investment company, engaged in financing instruments.', '指非以投資為業之商業，投資金融商品所產生之損失。'),
('7', '75', '755', 'Loss on disposal of assets', '處分資產損失', 'Loss from the sale, obsolescence, and loss of assets.', '凡因資產出售、報廢、及遺失等所發生之損失皆屬之。'),
('7', '75', '756', 'Loss on disposal of investments', '處分投資損失', 'Loss from disposal of short-term or long-term investments.', '凡因處分金融資產及長期投資所獲得之利益皆屬之。'),
('7', '78', '788', 'Other non-operating expenses', '其他營業外費損', 'Other non-operating expense that cannot be classified into the headings above', '凡不屬於以上各項之營業外費損皆屬之。'),

('A', 'A1', 'A11', 'Common revenue', '一般收入', '', ''),
('A', 'A2', 'A21', 'Investment revenue', '投資收益', '', ''),
('A', 'A8', 'A81', 'Other revenue', '其他收入', '', ''),

('B', 'B1', 'B11', 'Common and essential expenses', '一般及必要費用', '', ''),
('B', 'B1', 'B12', 'Common and essential expenses', '一般及必要費用', '', ''),
('B', 'B1', 'B13', 'Common and essential expenses', '一般及必要費用', '', ''),
('B', 'B1', 'B14', 'Common and essential expenses', '一般及必要費用', '', ''),
('B', 'B1', 'B15', 'Common and non-essential expenses', '一般及非必要費用', '', ''),
('B', 'B1', 'B16', 'Common and non-essential expenses', '一般及非必要費用', '', ''),
('B', 'B1', 'B17', 'Common and non-essential expenses', '一般及非必要費用', '', ''),
('B', 'B1', 'B18', 'Common and non-essential expenses', '一般及非必要費用', '', ''),
('B', 'B2', 'B21', 'Investment loss and liability expense', '投資損失及負債費用', '', ''),
('B', 'B8', 'B81', 'Other expense', '其他費用', '', '');

INSERT INTO ref_ledger_types (
	ledger_type_id, first_grade, second_grade, third_grade, type_name, type_name_zh
)
VALUES
-- 1: Assets, 11-12: Current assets
-- 111: Cash and cash equivalents
('1111', '1', '11', '111', 'Cash on hand', '現金及約當現金'),
('1113', '1', '11', '111', 'Cash in banks', '銀行存款'),
('1114', '1', '11', '111', 'Deposit account', '定期存款'),
('1117', '1', '11', '111', 'Cash Equivalents', '約當現金'),
('1118', '1', '11', '111', 'Other cash and cash equivalents', '其他現金及約當現金'),
-- 112: Short-term investments
('1121', '1', '11', '112', 'Financial assets at fair value through income statement', '公平價值變動列入損益之金融資產'),
('1122', '1', '11', '112', 'Financial assets in available-for-sale', '備供出售金融資產'),
('1123', '1', '11', '112', 'Financial assets in held-to-maturity', '持有至到期日金融資產'),
-- 113: Notes receivable
('1131', '1', '11', '113', 'Notes receivable', '應收票據'),
('1138', '1', '11', '113', 'Other notes receivable', '其他應收票據'),
('1139', '1', '11', '113', 'Allowance for uncollectible accounts - notes receivable', '備抵呆帳 - 應收票據'),
-- 121-122: Inventories
('1211', '1', '12', '121', 'Merchandise inventory', '商品存貨'),
('1219', '1', '12', '121', 'Allowance to reduce inventory to market', '備抵存貨跌價損失 (銷售)'),
('1221', '1', '12', '122', 'Finished goods', '製成品'),
('1226', '1', '12', '122', 'Raw materials', '原料'),
('1227', '1', '12', '122', 'Supplies', '物料'),
('1229', '1', '12', '122', 'Allowance to reduce inventory to market', '備抵存貨跌價損失 (製造)'),
-- 13: Funds and long-term investments
-- 131: Funds
('1311', '1', '13', '131', 'Redemption fund (or sinking fund)', '償債基金'),
('1312', '1', '13', '131', 'Fund for improvement and expansion', '改良及擴充基金'),
('1313', '1', '13', '131', 'Contingency fund', '意外損失準備基金'),
('1314', '1', '13', '131', 'Pension fund', '退休基金'),
('1318', '1', '13', '131', 'Other funds', '其他基金'),
-- 132-134: Long-term investments
('1321', '1', '13', '132', 'Financial assets at fair value through income statement - noncurrent', '公平價值變動列入損益之金融資產 - 非流動'),
('1322', '1', '13', '132', 'Financial assets in available-for-sale - noncurrent', '備供出售金融資產 - 非流動'),
('1323', '1', '13', '132', 'Financial asset in held-to-maturity - noncurrent', '持有至到期日金融資產 - 非流動'),
('1325', '1', '13', '132', 'Financial assets at cost - noncurrent', '以成本衡量之金融資產 - 非流動'),
('1341', '1', '13', '134', 'Longterm real estate investments', '長期不動產投資'),
('1345', '1', '13', '134', 'Cash surrender value of life insurance', '人壽保險現金解約現值'),
('1349', '1', '13', '134', 'Other long-term investments', '其他長期投資'),
-- 14-15: Fixed assets
-- 141: Land
('1411', '1', '14', '141', 'Land', '土地'),
('1417', '1', '14', '141', 'Revaluation increments - land ', '重估增值 - 土地'),
('1419', '1', '14', '141', 'Accumulated impairment - land ', '累計減損 - 土地'),
-- 143: Buildings
('1431', '1', '14', '143', 'Buildings', '房屋及建築'),
('1437', '1', '14', '143', 'Revaluation increments - buildings ', '重估增值 - 房屋及建築'),
('1438', '1', '14', '143', 'Accumulated depreciation - buildings', '累計折舊 - 房屋及建築'),
('1439', '1', '14', '143', 'Accumulated impairment - buildings', '累計減損 - 房屋及建築'),
-- 144-146: Machinery and equipment
('1441', '1', '14', '144', 'Machinery', '機(器)具'),
('1447', '1', '14', '144', 'Revaluation increments - machinery ', '重估增值 - 機(器)具'),
('1448', '1', '14', '144', 'Accumulated depreciation - machinery', '累計折舊 - 機(器)具'),
('1449', '1', '14', '144', 'Accumulated impairment - machinery', '累計減損 - 機(器)具'),
-- 151: Leased assets
('1511', '1', '15', '151', 'Leased assets', '租賃資產'),
('1518', '1', '15', '151', 'Accumulated depreciation - leased assets', '累計折舊 - 租賃資產'),
('1519', '1', '15', '151', 'Accumulated impairment - leased assets', '累計減損 - 租賃資產'),
-- 156: Construction in progress and prepayments for equipment
('1561', '1', '15', '156', 'Construction in progress', '未完工程'),
('1562', '1', '15', '156', 'Prepayments for equipment', '預付購置設備款'),
('1569', '1', '15', '156', 'Accumulated impairment - construction in progress', '累計減損 - 未完工程'),
-- 158: Miscellaneous property, plant, and equipment
('1581', '1', '15', '158', 'Miscellaneous property, plant, and equipment', '雜項固定資產'),
('1588', '1', '15', '158', 'Accumulated depreciation - miscellaneous property, plant, and equipment', '累計折舊 - 雜項固定資產'),
('1589', '1', '15', '158', 'Accumulated impairment - miscellaneous property, plant, and equipment', '累計減損 - 雜項固定資產'),
-- 17: Intangible assets
-- 171: Trademarks
('1711', '1', '17', '171', 'Trademarks', '商標權'),
('1717', '1', '17', '171', 'Revaluation increments - trademarks', '重估增值 - 商標權'),
('1719', '1', '17', '171', 'Accumulated impairment - trademarks', '累計減損 - 商標權'),
-- 172: Patents
('1721', '1', '17', '172', 'Patents', '專利權'),
('1727', '1', '17', '172', 'Revaluation increments - patents', '重估增值 - 專利權'),
('1729', '1', '17', '172', 'Accumulated impairment - patents', '累計減損 - 專利權'),
-- 174: Copyright
('1741', '1', '17', '174', 'Copyright', '著作權'),
('1749', '1', '17', '174', 'Accumulated impairment - copyright', '累計減損 - 著作權'),
-- 175: Computer software
('1751', '1', '17', '175', 'Computer software', '電腦軟體'),
('1758', '1', '17', '175', 'Accumulated amortization - computer software', '累計攤銷 - 電腦軟體'),
('1759', '1', '17', '175', 'Accumulated impairment - computer software', '累計減損 - 電腦軟體'),
-- 176: Goodwill
('1761', '1', '17', '176', 'Goodwill', '商譽'),
('1769', '1', '17', '176', 'Accumulated impairment - goodwill', '累計減損 - 商譽'),
-- 18: Other assets
-- 181: Deferred assets
('1812', '1', '18', '181', 'Long-term prepaid rent', '長期預付租金'),
('1813', '1', '18', '181', 'Long-term prepaid insurance', '長期預付保險費'),
('1815', '1', '18', '181', 'Prepaid pension cost', '預付退休金'),
('1818', '1', '18', '181', 'Other deferred assets', '其他遞延資產'),
-- 182: Idle assets
('1821', '1', '18', '182', 'Idle assets', '閒置資產'),
-- 185: Assets leased to others
('1851', '1', '18', '185', 'Assets leased to others', '出租資產'),
('1859', '1', '18', '185', 'Accumulated depreciation - assets leased to others', '累計折舊 - 出租資產'),
-- 188: Miscellaneous assets
('1888', '1', '18', '188', 'Miscellaneous assets - other', '雜項資產 - 其他'),

-- 2: Liabilities
-- 21-22: Current liabilities
-- 211: Short-term debt
('2112', '2', '21', '211', 'Bank loan', '短期借款 - 銀行'),
('2114', '2', '21', '211', 'short-term debt - owners', '短期借款 - 業主'),
('2115', '2', '21', '211', 'short-term debt - employees', '短期借款 - 員工'),
('2117', '2', '21', '211', 'short-term debt - related parties', '短期借款 - 關係人'),
('2118', '2', '21', '211', 'short-term debt - other', '短期借款 - 其他'),
-- 213: Notes payable
('2131', '2', '21', '213', 'Notes payable', '應付票據'),
('2137', '2', '21', '213', 'Notes payable - related parties', '應付票據 - 關係人'),
('2138', '2', '21', '213', 'Notes payable - other', '應付票據 - 其他'),
-- 217: Accrued expenses
('2171', '2', '21', '217', 'Accrued payroll', '應付薪工'),
('2172', '2', '21', '217', 'Rent payable', '應付租金'),
('2173', '2', '21', '217', 'Accrued interest payable', '應付利息'),
('2175', '2', '21', '217', 'Taxes payable - other', '應付稅捐 - 其他'),
('2178', '2', '21', '217', 'Other accrued expenses payable', '其他應付費用'),
-- 218-219: Other payables
('2191', '2', '21', '219', 'Dividend payable', '應付股利'),
('2192', '2', '21', '219', 'Bonus payable', '應付紅利'),
('2193', '2', '21', '219', 'Compensation payable to directors and supervisors', '應付董監事酬勞'),
('2198', '2', '21', '219', 'Other payables - other', '其他應付款 - 其他'),
-- 226: Unearned receipts
('2261', '2', '22', '226', 'Unearned sales revenue', '預收貨款'),
('2262', '2', '22', '226', 'Unearned revenue', '預收收入'),
('2268', '2', '22', '226', 'Other sales revenue', '其他預收款'),
-- 23: Long-term liabilities
-- 232: Long-term debt payable
('2321', '2', '23', '232', 'Long-term debt payable - bank', '長期借款 - 銀行'),
('2322', '2', '23', '232', 'Long-term debt payable - owners', '長期借款 - 業主'),
('2323', '2', '23', '232', 'Long-term debt payable - employees', '長期借款 - 員工'),
('2324', '2', '23', '232', 'Long-term debt payable - related parties', '長期借款 - 關係人'),
('2325', '2', '23', '232', 'Long-term debt payable - other', '長期借款 - 其他'),
-- 233: Long-term notes and accounts payable
('2331', '2', '23', '233', 'Long-term notes payable', '長期應付票據'),
-- 288: Miscellaneous liabilities
('2888', '2', '28', '288', 'Miscellaneous liabilities - other', '雜項資產 - 其他'),

-- 3: Owners’ equity
-- 31: Capital
-- 311: Capital
('3111', '3', '31', '311', 'Capital - common stock', '普通股股本'),
-- 33: Retained earnings
-- 335: Retained earnings unappropriated (or accumulated deficit)
('3351', '3', '33', '335', 'Accumulated profit or loss', '累積盈虧'),
('3353', '3', '33', '335', 'Net income or loss for current period', '本期損益'),

-- 4: Operating revenue
-- 41: Sales revenue
-- 411: Sales revenue
('4111', '4', '41', '411', 'Sales revenue', '銷貨收入'),
-- 46: Service revenue
-- 461: Service revenue
('4611', '4', '46', '461', 'Service revenue', '勞務收入'),
-- 48: Other operating revenue
-- 488: Other operating revenue
('4888', '4', '48', '488', 'Other operating revenue', '其他營業收入'),

-- 5: Operating costs
-- 51: Cost of goods sold
-- 511: Cost of goods sold
('5111', '5', '51', '511', 'Cost of goods sold', '銷貨成本'),
-- 512: Purchases
('5121', '5', '51', '512', 'Purchase', '進貨成本'),
('5122', '5', '51', '512', 'Purchase expenses', '進貨費用'),
-- 513: Material purchased
('5131', '5', '51', '513', 'Material purchased', '進料成本'),
('5132', '5', '51', '513', 'Charges on purchased material', '進料費用'),
-- 514: Direct labor
('5141', '5', '51', '514', 'Direct labor', '直接人工'),
-- 515-518: Manufacturing overhead
('5151', '5', '51', '515', 'Direct labor', '直接人工'),
('5152', '5', '51', '515', 'Rent expense', '租金支出'),
('5153', '5', '51', '515', 'Supplies expense', '文具用品'),
('5154', '5', '51', '515', 'Travelling expense', '旅費'),
('5155', '5', '51', '515', 'Shipping expenses', '運費'),
('5156', '5', '51', '515', 'Postage expense', '郵電費'),
('5157', '5', '51', '515', 'Repair(s) and maintenance expense', '修繕費'),
('5158', '5', '51', '515', 'Packing expense', '包裝費'),
('5161', '5', '51', '516', 'Utilities expense', '水電瓦斯費'),
('5162', '5', '51', '516', 'Insurance expense', '保險費'),
('5163', '5', '51', '516', 'Manufacturing overhead - outsourced', '外包加工費'),
('5166', '5', '51', '516', 'Taxes', '稅捐'),
('5168', '5', '51', '516', 'Depreciation expense', '折舊'),
('5169', '5', '51', '516', 'Various amortization', '各項耗竭及攤提'),
('5172', '5', '51', '517', 'Meal expense', '伙食費'),
('5173', '5', '51', '517', 'Employee benefits/welfare', '職工福利'),
('5176', '5', '51', '517', 'Training expense', '訓練費'),
('5177', '5', '51', '517', 'Indirect materials', '間接材料'),
('5188', '5', '51', '518', 'Other manufacturing expense', '其他製造費用'),
-- 56: Service costs
-- 561: Service costs
('5611', '5', '56', '561', 'Service costs', '勞務成本'),
-- 58: Other operating costs
-- 588: Other operating costs
('5888', '5', '58', '588', 'Other operating costs', '其他營業成本'),

-- 6: Operating expenses
-- 61: Selling expenses
-- 615-618: Selling expenses
('6151', '6', '61', '615', 'Payroll expense', '薪資支出'),
('6152', '6', '61', '615', 'Rent expense', '租金支出'),
('6153', '6', '61', '615', 'Supplies expense', '文具用品'),
('6154', '6', '61', '615', 'Travelling expense', '旅費'),
('6155', '6', '61', '615', 'Shipping expenses', '運費'),
('6156', '6', '61', '615', 'Postage expense', '郵電費'),
('6157', '6', '61', '615', 'Repair(s) and maintenance expense', '修繕費'),
('6159', '6', '61', '615', 'advertisement expense', '廣告費'),
('6161', '6', '61', '616', 'Utilities expense', '水電瓦斯費'),
('6162', '6', '61', '616', 'Insurance expense', '保險費'),
('6164', '6', '61', '616', 'Entertainment expense', '交際費'),
('6165', '6', '61', '616', 'Donation', '捐贈'),
('6166', '6', '61', '616', 'Taxes', '稅捐'),
('6167', '6', '61', '616', 'Loss on uncollectible accounts', '呆帳損失'),
('6168', '6', '61', '616', 'Depreciation expense', '折舊'),
('6169', '6', '61', '616', 'Various amortization', '各項耗竭及攤提'),
('6172', '6', '61', '617', 'Meal expense', '伙食費'),
('6173', '6', '61', '617', 'Employee benefits/welfare', '職工福利'),
('6175', '6', '61', '617', 'Commission expense', '佣金支出'),
('6176', '6', '61', '617', 'Training expense', '訓練費'),
('6188', '6', '61', '618', 'Other selling expense', '其他推銷費用'),
-- 62: General & administrative expenses
-- 625-628: General & administrative expenses
('6251', '6', '62', '625', 'Payroll expense', '薪資支出'),
('6252', '6', '62', '625', 'Rent expense', '租金支出'),
('6253', '6', '62', '625', 'Supplies expense', '文具用品'),
('6254', '6', '62', '625', 'Travelling expense', '旅費'),
('6255', '6', '62', '625', 'Shipping expenses', '運費'),
('6256', '6', '62', '625', 'Postage expense', '郵電費'),
('6257', '6', '62', '625', 'Repair(s) and maintenance expense', '修繕費'),
('6259', '6', '62', '625', 'advertisement expense', '廣告費'),
('6261', '6', '62', '626', 'Utilities expense', '水電瓦斯費'),
('6262', '6', '62', '626', 'Insurance expense', '保險費'),
('6264', '6', '62', '626', 'Entertainment expense', '交際費'),
('6265', '6', '62', '626', 'Donation', '捐贈'),
('6266', '6', '62', '626', 'Taxes', '稅捐'),
('6267', '6', '62', '626', 'Loss on uncollectible accounts', '呆帳損失'),
('6268', '6', '62', '626', 'Depreciation expense', '折舊'),
('6269', '6', '62', '626', 'Various amortization', '各項耗竭及攤提'),
('6271', '6', '62', '627', 'Loss on export sales', '外銷損失'),
('6272', '6', '62', '627', 'Meal expense', '伙食費'),
('6273', '6', '62', '627', 'Employee benefits/welfare', '職工福利'),
('6274', '6', '62', '627', 'Research and development expense', '研究發展費用'),
('6275', '6', '62', '627', 'Commission expense', '佣金支出'),
('6276', '6', '62', '627', 'Training expense', '訓練費'),
('6278', '6', '62', '627', 'Professional service fees', '勞務費'),
('6288', '6', '62', '628', 'Other general & administrative expenses', '其他管理及總務費用'),
-- 63: Research and development expenses
-- 635-638: Research and development expenses
('6351', '6', '63', '635', 'Payroll expense', '薪資支出'),
('6352', '6', '63', '635', 'Rent expense', '租金支出'),
('6353', '6', '63', '635', 'Supplies expense', '文具用品'),
('6354', '6', '63', '635', 'Travelling expense', '旅費'),
('6355', '6', '63', '635', 'Shipping expenses', '運費'),
('6356', '6', '63', '635', 'Postage expense', '郵電費'),
('6357', '6', '63', '635', 'Repair(s) and maintenance expense', '修繕費'),
('6361', '6', '63', '636', 'Utilities expense', '水電瓦斯費'),
('6362', '6', '63', '636', 'Insurance expense', '保險費'),
('6364', '6', '63', '636', 'Entertainment expense', '交際費'),
('6366', '6', '63', '636', 'Taxes', '稅捐'),
('6368', '6', '63', '636', 'Depreciation expense', '折舊'),
('6369', '6', '63', '636', 'Various amortization', '各項耗竭及攤提'),
('6372', '6', '63', '637', 'Meal expense', '伙食費'),
('6373', '6', '63', '637', 'Employee benefits/welfare', '職工福利'),
('6376', '6', '63', '637', 'Training expense', '訓練費'),
('6388', '6', '63', '638', 'Other research and development expenses', '其他研究發展費用'),

-- 7: Non-operating revenue and expenses
-- 71-74: Non-operating revenue
-- 711: Interest revenue
('7111', '7', '71', '711', 'Interest revenue/income', '利息收入'),
-- 714: Investment income
('7141', '7', '71', '714', 'Gain on valuation of financial asset', '金融資產評價利益'),
('7142', '7', '71', '714', 'Gain on valuation of financial liability', '金融負債評價利益'),
-- 716: Gain on disposal of investments
('7161', '7', '71', '716', 'Gain on disposal of investments', '處分投資收益'),
-- 717: Gain on disposal of assets
('7171', '7', '71', '717', 'Gain on disposal of assets', '處分資產溢價收入'),
-- 748: Other non-operating revenue
('7481', '7', '74', '748', 'Donation income', '捐贈收入'),
('7482', '7', '74', '748', 'Rent revenue/income', '租金收入'),
('7483', '7', '74', '748', 'Commission revenue/income', '佣金收入'),
('7484', '7', '74', '748', 'Revenue from sale of scraps', '出售下腳及廢料收入'),
('7485', '7', '74', '748', 'Gain on physical inventory', '存貨盤盈'),
('7487', '7', '74', '748', 'Gain on reversal of bad debts', '壞帳轉回利益'),
('7488', '7', '74', '748', 'Other non-operating revenue - other items', '其他營業外收益'),
-- 75-78: Non-operating expenses
-- 751: Interest expense
('7511', '7', '75', '751', 'Interest expense', '利息費用'),
-- 753: Investment loss
('7531', '7', '75', '753', 'Loss on valuation of financial asset', '金融資產評價損失'),
('7532', '7', '75', '753', 'Loss on valuation of financial liability', '金融負債評價損失'),
-- 755: Loss on disposal of assets
('7551', '7', '75', '755', 'Loss on disposal of assets', '處分資產損失'),
-- 756: Loss on disposal of investments
('7561', '7', '75', '756', 'Loss on disposal of investments', '處分投資損失'),
-- 788: Other non-operating expenses
('7881', '7', '78', '788', 'Loss on work stoppages', '停工損失'),
('7882', '7', '78', '788', 'Casualty loss', '災害損失'),
('7885', '7', '78', '788', 'Loss on physical inventory', '存貨盤損'),
('7888', '7', '78', '788', 'other non-operating expenses - other items', '其他營業外費損'),

-- A: Personal or household revenue
-- A11: Common revenue
('A111', 'A', 'A1', 'A11', 'Salary revenue', '薪資收入'),
('A112', 'A', 'A1', 'A11', 'Sales revenue', '銷貨收入'),
('A113', 'A', 'A1', 'A11', 'Service revenue', '勞務收入'),
('A114', 'A', 'A1', 'A11', 'Commission revenue/income', '佣金收入'),
-- A21: Investment revenue
('A211', 'A', 'A2', 'A21', 'Interest revenue/income', '利息收入'),
('A212', 'A', 'A2', 'A21', 'Gain on disposal of investments', '處分投資收入'),
('A213', 'A', 'A2', 'A21', 'Gain on disposal of assets', '處分資產收入'),
('A214', 'A', 'A2', 'A21', 'Rent revenue/income', '租金收入'),
('A215', 'A', 'A2', 'A21', 'Gain on valuation of financial asset', '金融資產評價利益'),
-- A81: Other revenue
('A811', 'A', 'A8', 'A81', 'Donation income', '捐贈收入'),
('A812', 'A', 'A8', 'A81', 'Revenue from sale of scraps', '出售雜物及廢料收入'),
('A813', 'A', 'A8', 'A81', 'Gain on cash inventory surplus', '現金盤盈'),
('A814', 'A', 'A8', 'A81', 'Gain on reversal of bad debts', '壞帳轉回利益'),
('A815', 'A', 'A8', 'A81', 'Other revenue - other items', '其他業外收益'),

-- B: Personal or household expenses
-- B1-B8: Personal or household expenses
-- B11-B14: Common and essential expenses
('B111', 'B', 'B1', 'B11', 'Meal expense', '伙食費'),
('B112', 'B', 'B1', 'B11', 'Transportation expense', '交通費'),
('B113', 'B', 'B1', 'B11', 'Educational expense', '教育費'),
('B114', 'B', 'B1', 'B11', 'Health and medical related expense', '健康及醫療費'),
('B115', 'B', 'B1', 'B11', 'Beauty expense', '美容保養費'),
('B116', 'B', 'B1', 'B11', 'Rent expense', '租金支出'),
('B117', 'B', 'B1', 'B11', 'Commission expense', '佣金支出'),
('B118', 'B', 'B1', 'B11', 'Repair(s) and maintenance expense', '修繕費'),
('B119', 'B', 'B1', 'B11', 'Utilities expense', '水電瓦斯網路費'),
('B121', 'B', 'B1', 'B12', 'Insurance expense', '保險費'),
('B122', 'B', 'B1', 'B12', 'Supplies expense', '文具用品'),
('B123', 'B', 'B1', 'B12', 'Taxes', '稅捐'),
-- B15-B18: Common and non-essential expenses
('B151', 'B', 'B1', 'B15', 'Apparel expense', '治裝費'),
('B152', 'B', 'B1', 'B15', 'Travelling expense', '旅費'),
('B153', 'B', 'B1', 'B15', 'Gift expense', '贈禮費用'),
('B154', 'B', 'B1', 'B15', 'Entertainment and social expense', '娛樂及交際費'),
('B155', 'B', 'B1', 'B15', 'Online service subscription fee', '線上服務訂閱費'),
('B156', 'B', 'B1', 'B15', 'Pet related expense', '寵物支出'),

-- B21: Investment loss and liability expense
('B211', 'B', 'B2', 'B21', 'Interest expense', '利息支出'),
('B212', 'B', 'B2', 'B21', 'Loss on disposal of investments', '處分投資損失'),
('B213', 'B', 'B2', 'B21', 'Loss on disposal of assets', '處分資產損失'),
('B214', 'B', 'B2', 'B21', 'Loss on valuation of financial asset', '金融資產評價損失'),
-- B81: Other expense
('B811', 'B', 'B8', 'B81', 'Donation expense', '捐贈費用'),
('B812', 'B', 'B8', 'B81', 'Loss on uncollectible accounts', '呆帳損失'),
('B813', 'B', 'B8', 'B81', 'Depreciation expense', '折舊'),
('B814', 'B', 'B8', 'B81', 'Various amortization', '各項耗竭及攤提'),
('B818', 'B', 'B8', 'B81', 'Other expense - other items', '其他支出');
